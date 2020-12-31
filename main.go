package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/chzyer/readline"
	"github.com/diamondburned/arikawa/discord"
	"github.com/diamondburned/arikawa/gateway"
	"github.com/diamondburned/arikawa/state"
	"github.com/pkg/errors"
)

var (
	token string
)

func init() {
	flag.StringVar(&token, "t", "", "Discord token to use")
}

func main() {
	flag.Parse()

	if token == "" {
		log.Fatalln("Missing token; declare with -t $TOKEN.")
	}

	s, err := connect(token)
	if err != nil {
		log.Fatalln("failed to connect:", err)
	}

	p, err := NewPrompt(s)
	if err != nil {
		log.Fatalln("failed to create prompt:", err)
	}

	if err := p.Run(); err != nil {
		log.Fatalln("failed to run readline:", err)
	}
}

func connect(token string) (*state.State, error) {
	s, err := state.New(token)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create state")
	}

	if err := s.Open(); err != nil {
		return nil, errors.Wrap(err, "failed to open")
	}

	return s, nil
}

type Prompt struct {
	*readline.Instance

	State *state.State

	mutex     sync.Mutex
	channelID discord.ChannelID
}

func NewPrompt(s *state.State) (*Prompt, error) {
	p, err := readline.New("> ")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read prompt")
	}

	prompt := Prompt{
		Instance: p,
		State:    s,
	}

	s.AddHandler(func(msg *gateway.MessageCreateEvent) {
		prompt.mutex.Lock()
		channelID := prompt.channelID
		prompt.mutex.Unlock()

		if channelID != msg.ChannelID {
			return
		}

		var name = msg.Author.Username
		if msg.Member != nil && msg.Member.Nick != "" {
			name = msg.Member.Nick
		}

		fmt.Fprintf(
			p, "[%s] %s: %s\n",
			msg.Timestamp.Format(time.Kitchen), name, msg.Content,
		)
	})

	s.AddHandler(func(msg *gateway.MessageUpdateEvent) {
		prompt.mutex.Lock()
		channelID := prompt.channelID
		prompt.mutex.Unlock()

		if channelID != msg.ChannelID {
			return
		}

		var name = msg.Author.Username
		if msg.Member != nil && msg.Member.Nick != "" {
			name = msg.Member.Nick
		}

		fmt.Fprintf(
			p, "[%s] %s: %s (edited)\n",
			msg.EditedTimestamp.Format(time.Kitchen), name, msg.Content,
		)
	})

	return &prompt, nil
}

func (p *Prompt) Run() error {
	p.writeLine("Welcome. Try typing '/help'.")
	for {
		line, err := p.Readline()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return errors.Wrap(err, "failed to read line")
		}

		if !strings.HasPrefix(line, "/") {
			p.sendMessage(line)
			continue
		}

		parts := strings.SplitN(line, " ", 2)
		k, v := strings.TrimPrefix(parts[0], "/"), ""
		if len(parts) == 2 {
			v = parts[1]
		}

		switch k {
		case "help":
			p.writeLine("Available commands: /list, /join [channelID].")
			p.writeLine("To send a message, type in directly.")
		case "list":
			p.list()
		case "join":
			p.join(v)
		}
	}
}

func (p *Prompt) writeError(err error) {
	io.WriteString(p, "Error: "+err.Error()+"\n")
}

func (p *Prompt) writeLine(line string) {
	io.WriteString(p, line+"\n")
}

func (p *Prompt) sendMessage(body string) {
	p.mutex.Lock()
	channelID := p.channelID
	p.mutex.Unlock()

	if !channelID.IsValid() {
		p.writeError(errors.New("not in any channel"))
		return
	}

	if body == "" {
		p.writeError(errors.New("missing message content"))
		return
	}

	_, err := p.State.SendText(channelID, body)
	if err != nil {
		p.writeError(err)
	}
}

func (p *Prompt) list() {
	guilds, err := p.State.Guilds()
	if err != nil {
		p.writeError(errors.Wrap(err, "failed to list all guilds"))
		return
	}

	for _, guild := range guilds {
		channels, err := p.State.Channels(guild.ID)
		if err != nil {
			p.writeError(errors.Wrap(err, "failed to get channels"))
			continue
		}

		fmt.Fprintf(p, "Guild %d: %q:\n", guild.ID, guild.Name)

		for _, ch := range channels {
			fmt.Fprintf(p, "\t- %d: %q\n", ch.ID, ch.Name)
		}
	}
}

func (p *Prompt) join(body string) {
	id, err := discord.ParseSnowflake(body)
	if err != nil {
		p.writeError(errors.Wrap(err, "failed to parse channel ID"))
		return
	}

	msgs, err := p.State.Messages(discord.ChannelID(id))
	if err != nil {
		p.writeError(errors.Wrap(err, "invalid channel"))
		return
	}

	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]

		fmt.Fprintf(
			p, "[%s] %s: %s\n",
			msg.Timestamp.Format(time.Kitchen), msg.Author.Username, msg.Content,
		)
	}

	p.mutex.Lock()
	p.channelID = discord.ChannelID(id)
	p.mutex.Unlock()
}
