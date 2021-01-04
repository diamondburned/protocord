package main

import (
	"strings"

	"github.com/c-bata/go-prompt"
)

func WrapAutoCompleter(state *State) prompt.Completer {
	return func(doc prompt.Document) []prompt.Suggest {
		return autocomplete(state, doc)
	}
}

// isFirstWord returns true if the current word is the first word.
func isFirstWord(doc prompt.Document) bool {
	return doc.FindStartOfPreviousWord() == 0
}

func autocomplete(state *State, doc prompt.Document) (suggestions []prompt.Suggest) {
	// Try to autocomplete command arguments.
	before := doc.TextBeforeCursor()
	fields := strings.Fields(before)
	if len(fields) == 0 {
		return nil
	}

	switch strings.TrimPrefix(fields[0], "/") {
	case "join":
		var search string
		if len(fields) > 1 {
			search = fields[1]
		}

		return searchAllChannels(state, search)
	}

	word := doc.GetWordBeforeCursor()
	if len(word) == 0 {
		return nil
	}

	// Prioritize command completion starting with a slash (/).
	if isFirstWord(doc) && word[0] == '/' {
		return prompt.FilterHasPrefix(commandSuggestions, word, true)
	}

	switch p, search := word[0], string(word[1:]); p {
	case '@':
		members, err := state.Store.Members(state.GuildID())
		if err != nil {
			return nil
		}

		lower := strings.ToLower(search)

		for _, m := range members {
			if match := hasPrefixes(lower, m.User.Username, m.Nick); match != "" {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        m.User.Mention(),
					Description: "@" + m.User.Username,
				})
			}
		}

		if len(suggestions) == 0 {
			state.MemberState.SearchMember(state.GuildID(), search)
		}

	case '#':
		channels, err := state.Store.Channels(state.GuildID())
		if err != nil {
			return nil
		}

		for _, ch := range channels {
			if match := hasPrefixes(search, ch.Name); match != "" {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        ch.Mention(),
					Description: "#" + ch.Name,
				})
			}
		}
	}

	return nil
}

func searchAllChannels(state *State, prefix string) (suggestions []prompt.Suggest) {
	guilds, err := state.Store.Guilds()
	if err != nil {
		return nil
	}

	for _, guild := range guilds {
		channels, err := state.Store.Channels(guild.ID)
		if err != nil {
			continue
		}

		for _, ch := range channels {
			if id := ch.ID.String(); strings.HasPrefix(id, prefix) {
				suggestions = append(suggestions, prompt.Suggest{
					Text:        id,
					Description: "#" + ch.Name,
				})
			}
		}
	}

	return
}

func hasPrefixes(word string, matches ...string) string {
	for _, match := range matches {
		if match != "" && strings.HasPrefix(strings.ToLower(match), word) {
			return match
		}
	}
	return ""
}
