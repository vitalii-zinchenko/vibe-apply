package main

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/lancekrogers/claude-code-go/pkg/claude"
)

// replyTimeout bounds how long a single Claude reply may take before we give up.
const replyTimeout = 3 * time.Minute

// persona is the system prompt that shapes the bot's replies. It fully replaces
// Claude Code's default (coding-agent) system prompt so the bot behaves like a
// chat companion, not a dev tool.
const persona = "You are Vibe Apply, a friendly and upbeat Discord bot. " +
	"Reply conversationally in one or two short sentences. " +
	"Use plain text only and never use any tools."

// discordMaxMessage is Discord's hard limit on a single message's length.
const discordMaxMessage = 2000

// discordResponder is the slice of *discordgo.Session that respond() needs.
// Keeping it an interface lets us unit-test the orchestration with a fake.
type discordResponder interface {
	ChannelTyping(channelID string, options ...discordgo.RequestOption) error
	ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
}

// replyGenerator turns a user's message into a reply. resumeSessionID is empty
// for a brand-new conversation; otherwise it continues that session. It returns
// the reply text and the session ID to resume next time.
type replyGenerator interface {
	Reply(ctx context.Context, userMessage, resumeSessionID string) (text, sessionID string, err error)
}

// sessionStore maps a Discord thread ID to the Claude session ID that backs it,
// so each thread is one continuous Claude conversation. In-memory only: mappings
// are lost on restart (older threads then simply start a fresh conversation).
type sessionStore struct {
	mu       sync.Mutex
	byThread map[string]string
}

func newSessionStore() *sessionStore {
	return &sessionStore{byThread: make(map[string]string)}
}

func (s *sessionStore) get(threadID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.byThread[threadID]
}

func (s *sessionStore) set(threadID, sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.byThread[threadID] = sessionID
}

// respond generates a reply for one thread and posts it, keeping the thread's
// Claude session up to date. On any generation error it logs and posts nothing
// (better silent than spamming errors into the channel).
func respond(ctx context.Context, resp discordResponder, gen replyGenerator, store *sessionStore, threadID, userMessage string) {
	_ = resp.ChannelTyping(threadID) // show "Vibe Apply is typing…" while we think

	resumeID := store.get(threadID) // "" -> fresh conversation
	text, sessionID, err := gen.Reply(ctx, userMessage, resumeID)
	if err != nil {
		log.Printf("reply generation failed for thread %s: %v", threadID, err)
		return
	}
	if sessionID != "" {
		store.set(threadID, sessionID)
	}
	if _, err := resp.ChannelMessageSend(threadID, truncateMessage(text)); err != nil {
		log.Printf("failed to send reply in thread %s: %v", threadID, err)
	}
}

// handleReply runs one reply end-to-end off the Gateway goroutine: it bounds
// concurrency via sem (so a burst of messages can't spawn unbounded Claude
// processes) and applies a timeout, then delegates to respond.
func handleReply(resp discordResponder, gen replyGenerator, store *sessionStore, sem chan struct{}, threadID, userMessage string) {
	sem <- struct{}{}
	defer func() { <-sem }()

	ctx, cancel := context.WithTimeout(context.Background(), replyTimeout)
	defer cancel()
	respond(ctx, resp, gen, store, threadID, userMessage)
}

// buildUserMessage renders the human's message into the prompt Claude sees,
// including who is speaking. Falls back gracefully for empty or authorless
// messages (e.g. image-only posts).
func buildUserMessage(m *discordgo.Message) string {
	name := "someone"
	if m.Author != nil && m.Author.Username != "" {
		name = m.Author.Username
	}
	content := strings.TrimSpace(m.Content)
	if content == "" {
		content = "(no text)"
	}
	return fmt.Sprintf("%s said: %s", name, content)
}

// truncateMessage keeps a reply within Discord's per-message length limit.
func truncateMessage(s string) string {
	r := []rune(s)
	if len(r) <= discordMaxMessage {
		return s
	}
	return string(r[:discordMaxMessage-1]) + "…"
}

// claudeReplyGenerator is the production replyGenerator: it drives the local
// Claude Code CLI (under the user's subscription) with tools and MCP disabled.
type claudeReplyGenerator struct {
	client *claude.ClaudeClient
}

func newClaudeReplyGenerator() *claudeReplyGenerator {
	return &claudeReplyGenerator{client: claude.NewClient("claude")}
}

func (g *claudeReplyGenerator) Reply(ctx context.Context, userMessage, resumeSessionID string) (string, string, error) {
	opts := &claude.RunOptions{
		Format:          claude.JSONOutput,
		SystemPrompt:    persona,
		AllowedTools:    []string{}, // no tools: untrusted Discord input must not drive tool use
		StrictMCPConfig: true,       // ignore all MCP servers for speed and safety
		ResumeID:        resumeSessionID,
	}
	res, err := g.client.RunPromptCtx(ctx, userMessage, opts)
	if err != nil {
		return "", "", err
	}
	if res.IsError {
		return "", res.SessionID, fmt.Errorf("claude reported an error: %s", res.Result)
	}
	text := strings.TrimSpace(res.Result)
	if text == "" {
		return "", res.SessionID, fmt.Errorf("claude returned an empty reply")
	}
	return text, res.SessionID, nil
}
