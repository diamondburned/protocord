package main

import (
	"sync"

	"github.com/diamondburned/arikawa/discord"
	"github.com/diamondburned/arikawa/state"
	"github.com/diamondburned/ningen"
	"github.com/pkg/errors"
)

type State struct {
	*ningen.State
	mutex       sync.RWMutex
	guildID     discord.GuildID
	channelID   discord.ChannelID
	channelName string
}

func connect(token string) (*State, error) {
	s, err := state.New(token)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create state")
	}

	n, err := ningen.FromState(s)
	if err != nil {
		return nil, errors.Wrap(err, "failed to wrap ningen")
	}

	if err := n.Open(); err != nil {
		return nil, errors.Wrap(err, "failed to open")
	}

	return &State{State: n}, nil
}

func (s *State) ChannelName() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.channelName
}

func (s *State) ChannelID() discord.ChannelID {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.channelID
}

func (s *State) GuildID() discord.GuildID {
	s.mutex.RLock()
	defer s.mutex.RUnlock()

	return s.guildID
}
