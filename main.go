package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/c-bata/go-prompt"
	"github.com/diamondburned/arikawa/api"
	"github.com/diamondburned/arikawa/discord"
	"github.com/diamondburned/arikawa/gateway"
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

	p := NewPrompt(s)
	p.Run()
}

type Prompt struct {
	writeMu sync.Mutex
	writer  prompt.ConsoleWriter

	prompt *prompt.Prompt
	state  *State

	lastTyped time.Time
}

func NewPrompt(s *State) *Prompt {
	p := Prompt{
		writer: prompt.NewStdoutWriter(),
		state:  s,
	}

	p.prompt = prompt.New(
		p.execute, WrapAutoCompleter(p.state),
		prompt.OptionTitle("protocord"),
		prompt.OptionPrefix("> "),
		prompt.OptionLivePrefix(p.prefix),
		prompt.OptionWriter(p.writer),
		prompt.OptionSetExitCheckerOnInput(p.onChange),
	)

	s.AddHandler(func(msg *gateway.MessageCreateEvent) {
		if p.state.ChannelID() != msg.ChannelID {
			return
		}

		var name = msg.Author.Username
		if msg.Member != nil && msg.Member.Nick != "" {
			name = msg.Member.Nick
		}

		p.Writelnf(
			"[%s] %s: %s",
			msg.Timestamp.Time().Local().Format(time.Kitchen),
			name, msg.Content,
		)
	})

	s.AddHandler(func(msg *gateway.MessageUpdateEvent) {
		if p.state.ChannelID() != msg.ChannelID {
			return
		}

		var name = msg.Author.Username
		if msg.Member != nil && msg.Member.Nick != "" {
			name = msg.Member.Nick
		}

		p.Writelnf(
			"[%s] %s: %s (edited)",
			msg.EditedTimestamp.Time().Local().Format(time.Kitchen),
			name, msg.Content,
		)
	})

	s.AddHandler(func(t *gateway.TypingStartEvent) {
		if p.state.ChannelID() != t.ChannelID {
			return
		}

		// Lazy mode.
		if t.Member != nil {
			var name = t.Member.Nick
			if name == "" {
				name = t.Member.User.Username
			}

			p.WriteColorlnf(
				prompt.LightGray,
				"*%s is typing.*", name,
			)
		}
	})

	return &p
}

func (p *Prompt) Run() { p.prompt.Run() }

var commandSuggestions = []prompt.Suggest{
	{Text: "/help", Description: "Print the help message."},
	{Text: "/quit", Description: "Quit."},
	{Text: "/list", Description: "List all servers."},
	{Text: "/join", Description: "Join a channel."},
	{Text: "/join-invite", Description: "Join a guild using an invite code."},
	{Text: "/create-invite", Description: "Create an invite code to the current channel."},
	{Text: "/create-guild", Description: "Create a new guild"},
	{Text: "/create-channel", Description: "Create a new channel"},
}

func (p *Prompt) execute(line string) {
	if !strings.HasPrefix(line, "/") {
		p.sendMessage(line)
		return
	}

	parts := strings.SplitN(line, " ", 2)
	k, v := strings.TrimPrefix(parts[0], "/"), ""
	if len(parts) == 2 {
		v = parts[1]
	}

	switch k {
	case "help":
		p.WriteLine("To send a message, type in directly.")
		p.WriteLine("Available commands:")
		p.WriteLine("	/list")
		p.WriteLine("	/quit")
		p.WriteLine("	/join <channelID>")
		p.WriteLine("	/join-invite <inviteCode>")
		p.WriteLine("	/create-invite [channelID] [json:createInviteData]")
		p.WriteLine("	/create-guild")
		p.WriteLine("	/create-channel")

	case "list":
		p.list()
	case "quit":
		// refer to shouldExit().
	case "join":
		p.join(v)
	case "join-invite":
		p.joinInvite(v)
	case "create-invite":
		p.createInvite(v)
	case "create-guild":
		p.createGuild(v)
	case "create-channel":
		p.createChannel(v)
	}
}

func (p *Prompt) prefix() (prefix string, use bool) {
	name := p.state.ChannelName()
	return fmt.Sprintf("[#%s] ", name), name != ""
}

const typeFrequency = 8 * time.Second

func (p *Prompt) onChange(in string, broke bool) (quit bool) {
	// go-prompt is an awful library.
	if broke {
		return in == "/quit"
	}

	if id := p.state.ChannelID(); id.IsValid() {
		now := time.Now()

		if p.lastTyped.Add(typeFrequency).Before(now) {
			p.lastTyped = now
			go p.state.Typing(id)
		}
	}

	return false
}

func (p *Prompt) WriteError(err error) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	p.writer.SetColor(prompt.DarkRed, prompt.DefaultColor, false)
	p.writer.WriteStr("Error: " + err.Error() + "\n")
	p.writer.SetColor(prompt.DefaultColor, prompt.DefaultColor, false)
}

func (p *Prompt) WriteLine(line string) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	p.writer.WriteStr(line + "\n")
}

func (p *Prompt) Writelnf(f string, v ...interface{}) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	p.writer.WriteStr(fmt.Sprintf(f+"\n", v...))
}

func (p *Prompt) WriteColorlnf(color prompt.Color, f string, v ...interface{}) {
	p.writeMu.Lock()
	defer p.writeMu.Unlock()

	p.writer.SetColor(color, prompt.DefaultColor, false)
	p.writer.WriteStr(fmt.Sprintf(f+"\n", v...))
	p.writer.SetColor(prompt.DefaultColor, prompt.DefaultColor, false)
}

func (p *Prompt) sendMessage(body string) {
	channelID := p.state.ChannelID()
	if !channelID.IsValid() {
		p.WriteError(errors.New("not in any channel"))
		return
	}

	_, err := p.state.SendText(channelID, body)
	if err != nil {
		p.WriteError(err)
	}
}

func (p *Prompt) list() {
	guilds, err := p.state.Guilds()
	if err != nil {
		p.WriteError(errors.Wrap(err, "failed to list all guilds"))
		return
	}

	for _, guild := range guilds {
		channels, err := p.state.Channels(guild.ID)
		if err != nil {
			p.WriteError(errors.Wrap(err, "failed to get channels"))
			continue
		}

		p.Writelnf("Guild %d: %q:", guild.ID, guild.Name)

		for _, ch := range channels {
			p.Writelnf("    - %d: %q", ch.ID, ch.Name)
		}
	}
}

func (p *Prompt) join(body string) {
	id, err := discord.ParseSnowflake(body)
	if err != nil {
		p.WriteError(errors.Wrap(err, "failed to parse channel ID"))
		return
	}

	ch, err := p.state.Channel(discord.ChannelID(id))
	if err != nil {
		p.WriteError(errors.Wrap(err, "invalid channel"))
		return
	}

	msgs, err := p.state.Messages(ch.ID)
	if err != nil {
		p.WriteError(errors.Wrap(err, "failed to get messages"))
		return
	}

	for i := len(msgs) - 1; i >= 0; i-- {
		msg := msgs[i]

		p.Writelnf(
			"[%s] %s: %s",
			msg.Timestamp.Time().Local().Format(time.Kitchen),
			msg.Author.Username, msg.Content,
		)
	}

	p.state.mutex.Lock()
	p.state.channelName = ch.Name
	p.state.channelID = ch.ID
	p.state.guildID = ch.GuildID
	p.state.mutex.Unlock()

	if ch.GuildID.IsValid() {
		p.state.MemberState.Subscribe(ch.GuildID)
	}
}

func (p *Prompt) joinInvite(body string) {
	joined, err := p.state.JoinInvite(body)
	if err != nil {
		p.WriteError(errors.Wrap(err, "failed to join invite"))
		return
	}

	if joined.Channel.ID.IsValid() {
		p.state.mutex.Lock()
		p.state.channelName = joined.Channel.Name
		p.state.channelID = joined.Channel.ID
		p.state.guildID = joined.Guild.ID
		p.state.mutex.Unlock()
	}

	p.Writelnf(
		"Joined guild %q (%d) into channel %q (%d).",
		joined.Guild.Name, joined.Guild.ID, joined.Channel.Name, joined.Channel.ID,
	)
}

func (p *Prompt) createInvite(body string) {
	parts := strings.SplitN(body, " ", 2)
	inviteChannel := p.state.ChannelID()

	var inviteData api.CreateInviteData

	if parts[0] != "" {
		id, err := discord.ParseSnowflake(parts[0])
		if err != nil {
			p.WriteError(errors.Wrap(err, "failed to parse channelID"))
			return
		}
		inviteChannel = discord.ChannelID(id)
	}

	if len(parts) == 2 {
		if err := json.Unmarshal([]byte(parts[1]), &inviteData); err != nil {
			p.WriteError(errors.Wrap(err, "failed to parse invite data JSON"))
			return
		}
	}

	inv, err := p.state.CreateInvite(inviteChannel, inviteData)
	if err != nil {
		p.WriteError(errors.Wrap(err, "failed to create invite"))
		return
	}

	p.Writelnf("Invite created: %q", inv.Code)
}

func (p *Prompt) createGuild(body string) {
	g, err := p.state.CreateGuild(api.CreateGuildData{
		Name: body,
	})
	if err != nil {
		p.WriteError(errors.Wrap(err, "failed to create guild"))
		return
	}

	p.Writelnf("Guild with ID %d created", g.ID)
}

func (p *Prompt) createChannel(body string) {
	ch, err := p.state.CreateChannel(p.state.GuildID(), api.CreateChannelData{
		Name: body,
	})
	if err != nil {
		p.WriteError(errors.Wrap(err, "failed to create channel"))
		return
	}

	p.Writelnf("Channel with ID %d created", ch.ID)
}
