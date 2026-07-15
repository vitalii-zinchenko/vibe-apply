package main

import (
	"testing"

	"github.com/bwmarrin/discordgo"
)

const (
	testChannelID = "target-channel-123"
	testBotID     = "bot-user-999"
)

// newResolver builds a channel-lookup function backed by a fixed map, mimicking
// what the live handler does via Discord's state/REST.
func newResolver(channels map[string]*discordgo.Channel) func(string) *discordgo.Channel {
	return func(id string) *discordgo.Channel {
		return channels[id]
	}
}

func threadUnder(id, parentID string) *discordgo.Channel {
	return &discordgo.Channel{
		ID:       id,
		ParentID: parentID,
		Type:     discordgo.ChannelTypeGuildPublicThread,
	}
}

func TestDecideAction(t *testing.T) {
	// A thread that hangs off the watched channel, and one that hangs off a
	// different channel, plus a plain (non-thread) channel.
	threadInTarget := threadUnder("thread-in-target", testChannelID)
	threadElsewhere := threadUnder("thread-elsewhere", "other-channel")
	plainChannel := &discordgo.Channel{ID: "plain-other", Type: discordgo.ChannelTypeGuildText}

	resolve := newResolver(map[string]*discordgo.Channel{
		threadInTarget.ID:  threadInTarget,
		threadElsewhere.ID: threadElsewhere,
		plainChannel.ID:    plainChannel,
	})

	human := &discordgo.User{ID: "human-1", Bot: false}
	otherBot := &discordgo.User{ID: "other-bot-2", Bot: true}
	self := &discordgo.User{ID: testBotID, Bot: true}

	tests := []struct {
		name string
		msg  *discordgo.Message
		want action
	}{
		{
			name: "human posts in watched channel -> create a thread",
			msg:  &discordgo.Message{ChannelID: testChannelID, Author: human},
			want: actionCreateThread,
		},
		{
			name: "human posts in a thread under the watched channel -> reply in thread",
			msg:  &discordgo.Message{ChannelID: threadInTarget.ID, Author: human},
			want: actionReplyInThread,
		},
		{
			name: "human posts in a thread under a DIFFERENT channel -> ignore",
			msg:  &discordgo.Message{ChannelID: threadElsewhere.ID, Author: human},
			want: actionIgnore,
		},
		{
			name: "human posts in some other non-thread channel -> ignore",
			msg:  &discordgo.Message{ChannelID: plainChannel.ID, Author: human},
			want: actionIgnore,
		},
		{
			name: "bot's own greeting in the watched channel -> ignore (no thread spam)",
			msg:  &discordgo.Message{ChannelID: testChannelID, Author: self},
			want: actionIgnore,
		},
		{
			name: "bot's own greeting inside a thread -> ignore (no reply loop)",
			msg:  &discordgo.Message{ChannelID: threadInTarget.ID, Author: self},
			want: actionIgnore,
		},
		{
			name: "another bot in a thread under the watched channel -> ignore",
			msg:  &discordgo.Message{ChannelID: threadInTarget.ID, Author: otherBot},
			want: actionIgnore,
		},
		{
			name: "nil author -> ignore, no panic",
			msg:  &discordgo.Message{ChannelID: testChannelID, Author: nil},
			want: actionIgnore,
		},
		{
			name: "message in an unknown/unresolvable channel -> ignore, no panic",
			msg:  &discordgo.Message{ChannelID: "never-seen-this-channel", Author: human},
			want: actionIgnore,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideAction(tt.msg, testChannelID, testBotID, resolve)
			if got != tt.want {
				t.Errorf("decideAction() = %v, want %v", got, tt.want)
			}
		})
	}
}
