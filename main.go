package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

// threadName is the fixed title for every thread the bot creates.
const threadName = "Vibe Apply"

// threadArchiveMinutes is how long a thread stays active without messages
// before Discord auto-archives it (1440 = 1 day). New messages un-archive it.
const threadArchiveMinutes = 1440

// maxConcurrentReplies caps how many Claude subprocesses run at once so a burst
// of messages can't overwhelm the machine.
const maxConcurrentReplies = 3

// action is what the bot should do in response to a message.
type action int

const (
	actionIgnore action = iota
	actionCreateThread  // human posted in the watched channel: open a thread + greet
	actionReplyInThread // human posted inside a thread under the watched channel: greet
)

func (a action) String() string {
	switch a {
	case actionCreateThread:
		return "createThread"
	case actionReplyInThread:
		return "replyInThread"
	default:
		return "ignore"
	}
}

func main() {
	// Load a local .env if present. Missing file is fine — real env vars still work.
	_ = godotenv.Load()

	token := flag.String("token", os.Getenv("DISCORD_TOKEN"), "Discord bot token (or set DISCORD_TOKEN)")
	channelID := flag.String("channel", os.Getenv("DISCORD_CHANNEL_ID"), "Target channel ID to monitor (or set DISCORD_CHANNEL_ID)")
	flag.Parse()

	if *token == "" {
		log.Fatal("missing bot token: pass -token or set DISCORD_TOKEN")
	}
	if *channelID == "" {
		log.Fatal("missing channel ID: pass -channel or set DISCORD_CHANNEL_ID")
	}

	dg, err := discordgo.New("Bot " + *token)
	if err != nil {
		log.Fatalf("failed to create Discord session: %v", err)
	}

	// We need guild message events, and Message Content is a privileged intent
	// that must ALSO be enabled in the Discord Developer Portal, otherwise
	// message content arrives empty.
	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentMessageContent

	// Isolate the Claude subprocess: run it from a temp dir outside $HOME so it
	// doesn't auto-load project/user CLAUDE.md files (which would give it an
	// unrelated persona). Combined with StrictMCPConfig + no tools in llm.go,
	// this keeps replies fast, on-persona, and safe. (.env was loaded above.)
	if workDir, err := os.MkdirTemp("", "vibe-apply-claude"); err != nil {
		log.Printf("warning: could not create isolated work dir: %v", err)
	} else if err := os.Chdir(workDir); err != nil {
		log.Printf("warning: could not switch to isolated work dir: %v", err)
	}

	store := newSessionStore()
	gen := newClaudeReplyGenerator()
	sem := make(chan struct{}, maxConcurrentReplies)

	dg.AddHandler(func(s *discordgo.Session, m *discordgo.MessageCreate) {
		// resolve looks up a channel by ID: cached state first, REST as fallback.
		// It is how we tell whether a message came from a thread under the
		// watched channel. Returns nil if the channel can't be found.
		resolve := func(id string) *discordgo.Channel {
			if ch, err := s.State.Channel(id); err == nil {
				return ch
			}
			ch, err := s.Channel(id)
			if err != nil {
				return nil
			}
			return ch
		}

		switch decideAction(m.Message, *channelID, s.State.User.ID, resolve) {
		case actionCreateThread:
			thread, err := s.MessageThreadStart(m.ChannelID, m.ID, threadName, threadArchiveMinutes)
			if err != nil {
				log.Printf("failed to start thread on message %s: %v", m.ID, err)
				return
			}
			// Generate off the Gateway goroutine so a slow reply never blocks
			// heartbeats or other events.
			go handleReply(s, gen, store, sem, thread.ID, buildUserMessage(m.Message))
		case actionReplyInThread:
			go handleReply(s, gen, store, sem, m.ChannelID, buildUserMessage(m.Message))
		}
	})

	if err := dg.Open(); err != nil {
		log.Fatalf("failed to open Discord connection: %v", err)
	}
	defer dg.Close()

	log.Printf("Vibe Apply is listening on channel %s. Press Ctrl+C to stop.", *channelID)

	// Block until interrupted, then shut down cleanly.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop
	log.Println("Shutting down.")
}

// decideAction is the single source of truth for how the bot reacts to a
// message. It is also the loop-safety guard: the self and bot checks are what
// stop the bot from reacting to its own greetings.
//
// resolve returns the channel for a given ID (or nil) so we can tell whether a
// message came from a thread under the watched channel. Keeping it a parameter
// makes this function pure and unit-testable without a live session.
func decideAction(m *discordgo.Message, targetChannelID, botUserID string, resolve func(string) *discordgo.Channel) action {
	if m.Author == nil {
		return actionIgnore // system messages have no author
	}
	if m.Author.ID == botUserID {
		return actionIgnore // never react to ourselves
	}
	if m.Author.Bot {
		return actionIgnore // ignore other bots and webhooks
	}

	// A human posting directly in the watched channel starts a new thread.
	if m.ChannelID == targetChannelID {
		return actionCreateThread
	}

	// Otherwise, greet only if the message is in a thread that hangs off the
	// watched channel.
	ch := resolve(m.ChannelID)
	if ch != nil && ch.IsThread() && ch.ParentID == targetChannelID {
		return actionReplyInThread
	}
	return actionIgnore
}
