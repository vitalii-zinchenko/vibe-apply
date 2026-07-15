package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/bwmarrin/discordgo"
)

// --- fakes ---------------------------------------------------------------

type fakeGenerator struct {
	text      string
	session   string
	err       error
	gotResume []string // resume IDs it was called with, in order
}

func (f *fakeGenerator) Reply(_ context.Context, _ string, resumeSessionID string) (string, string, error) {
	f.gotResume = append(f.gotResume, resumeSessionID)
	return f.text, f.session, f.err
}

type sentMessage struct{ channel, content string }

type fakeResponder struct {
	typed   []string
	sent    []sentMessage
	sendErr error
}

func (f *fakeResponder) ChannelTyping(channelID string, _ ...discordgo.RequestOption) error {
	f.typed = append(f.typed, channelID)
	return nil
}

func (f *fakeResponder) ChannelMessageSend(channelID, content string, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	f.sent = append(f.sent, sentMessage{channelID, content})
	return &discordgo.Message{ID: "sent"}, f.sendErr
}

// --- sessionStore --------------------------------------------------------

func TestSessionStore(t *testing.T) {
	s := newSessionStore()
	if got := s.get("unknown"); got != "" {
		t.Errorf("get(unknown) = %q, want empty", got)
	}
	s.set("thread-1", "sess-a")
	if got := s.get("thread-1"); got != "sess-a" {
		t.Errorf("get after set = %q, want sess-a", got)
	}
	s.set("thread-1", "sess-b") // overwrite (session id rotates each turn)
	if got := s.get("thread-1"); got != "sess-b" {
		t.Errorf("get after overwrite = %q, want sess-b", got)
	}
}

// --- respond -------------------------------------------------------------

func TestRespondNewThreadStoresSession(t *testing.T) {
	store := newSessionStore()
	gen := &fakeGenerator{text: "hello there", session: "sess-1"}
	resp := &fakeResponder{}

	respond(context.Background(), resp, gen, store, "thread-1", "user says hi")

	if len(gen.gotResume) != 1 || gen.gotResume[0] != "" {
		t.Errorf("expected generator called once with empty resume id, got %v", gen.gotResume)
	}
	if got := store.get("thread-1"); got != "sess-1" {
		t.Errorf("session not stored: got %q, want sess-1", got)
	}
	if len(resp.sent) != 1 || resp.sent[0] != (sentMessage{"thread-1", "hello there"}) {
		t.Errorf("unexpected sends: %v", resp.sent)
	}
	if len(resp.typed) != 1 || resp.typed[0] != "thread-1" {
		t.Errorf("expected typing indicator on thread-1, got %v", resp.typed)
	}
}

func TestRespondFollowUpResumesStoredSession(t *testing.T) {
	store := newSessionStore()
	store.set("thread-1", "sess-1") // an existing conversation
	gen := &fakeGenerator{text: "welcome back", session: "sess-2"}
	resp := &fakeResponder{}

	respond(context.Background(), resp, gen, store, "thread-1", "another message")

	if len(gen.gotResume) != 1 || gen.gotResume[0] != "sess-1" {
		t.Errorf("expected resume with sess-1, got %v", gen.gotResume)
	}
	if got := store.get("thread-1"); got != "sess-2" {
		t.Errorf("session not updated: got %q, want sess-2", got)
	}
	if len(resp.sent) != 1 {
		t.Errorf("expected one send, got %v", resp.sent)
	}
}

func TestRespondGeneratorErrorDoesNotSendOrStore(t *testing.T) {
	store := newSessionStore()
	gen := &fakeGenerator{err: errors.New("claude blew up"), session: "should-not-store"}
	resp := &fakeResponder{}

	respond(context.Background(), resp, gen, store, "thread-1", "hi")

	if len(resp.sent) != 0 {
		t.Errorf("expected no message sent on error, got %v", resp.sent)
	}
	if got := store.get("thread-1"); got != "" {
		t.Errorf("expected no session stored on error, got %q", got)
	}
}

// --- helpers -------------------------------------------------------------

func TestBuildUserMessage(t *testing.T) {
	got := buildUserMessage(&discordgo.Message{Author: &discordgo.User{Username: "zinjvi"}, Content: "hey there"})
	if !strings.Contains(got, "zinjvi") || !strings.Contains(got, "hey there") {
		t.Errorf("expected name and content in %q", got)
	}

	empty := buildUserMessage(&discordgo.Message{Author: &discordgo.User{Username: "zinjvi"}, Content: "   "})
	if strings.Contains(empty, "zinjvi") && !strings.Contains(empty, "(no text)") {
		t.Errorf("expected empty-content fallback in %q", empty)
	}

	nilAuthor := buildUserMessage(&discordgo.Message{Content: "hi"})
	if nilAuthor == "" {
		t.Error("expected non-empty message for nil author")
	}
}

func TestTruncateMessage(t *testing.T) {
	if got := truncateMessage("short"); got != "short" {
		t.Errorf("short message changed: %q", got)
	}
	long := strings.Repeat("x", 5000)
	got := truncateMessage(long)
	if len([]rune(got)) > discordMaxMessage {
		t.Errorf("truncated length %d exceeds limit %d", len([]rune(got)), discordMaxMessage)
	}
}
