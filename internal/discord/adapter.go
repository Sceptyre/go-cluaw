package discord

import (
	"fmt"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/sceptye/go-cluaw/internal/agent"
	"github.com/sceptye/go-cluaw/internal/config"
	"github.com/sceptye/go-cluaw/internal/logger"
)

type Adapter struct {
	cfg       config.DiscordConfig
	session   *discordgo.Session
	agentLoop *agent.Agent
	log       *logger.Logger
}

func New(cfg config.DiscordConfig, agentLoop *agent.Agent, log *logger.Logger) (*Adapter, error) {
	token := os.ExpandEnv(cfg.Token)
	if token == "" {
		return nil, fmt.Errorf("DISCORD_BOT_TOKEN not set")
	}

	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("create Discord session: %w", err)
	}

	return &Adapter{
		cfg:       cfg,
		session:   dg,
		agentLoop: agentLoop,
		log:       log,
	}, nil
}

func (a *Adapter) Connect() error {
	a.session.AddHandler(a.handleMessage)

	a.session.Identify.Intents = discordgo.IntentsMessageContent | discordgo.IntentsGuildMessages

	if err := a.session.Open(); err != nil {
		return fmt.Errorf("open websocket: %w", err)
	}

	a.log.Info("Connected to Discord")
	return nil
}

func (a *Adapter) Disconnect() {
	if a.session != nil {
		a.session.Close()
	}
}

// Session returns the underlying Discord session for use by other components.
func (a *Adapter) Session() *discordgo.Session {
	return a.session
}

func (a *Adapter) handleMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	shouldRespond := false
	content := m.Content

	if a.cfg.Trigger.Mentions {
		content = strings.TrimSpace(strings.ReplaceAll(content, "<@"+s.State.User.ID+">", ""))
		content = strings.TrimSpace(strings.ReplaceAll(content, "<@!"+s.State.User.ID+">", ""))
		for _, mention := range m.Mentions {
			if mention.ID == s.State.User.ID {
				shouldRespond = true
				break
			}
		}
	}

	if a.cfg.Trigger.DirectMessages {
		ch, err := s.Channel(m.ChannelID)
		if err == nil && ch.Type == discordgo.ChannelTypeDM {
			shouldRespond = true
		}
	}

	if !shouldRespond && !a.cfg.Trigger.Mentions {
		for _, mention := range m.Mentions {
			if mention.ID == s.State.User.ID {
				shouldRespond = true
				break
			}
		}
	}

	if !shouldRespond && content == "" {
		return
	}

	msg := content
	if msg == "" {
		msg = "(ping)"
	}

	response, err := a.agentLoop.Process(m.Author.ID, msg)
	if err != nil {
		a.log.Error("Agent error: %v", err)
		response = "I encountered an error processing that."
	}

	if response != "" {
		_, err = s.ChannelMessageSend(m.ChannelID, response)
		if err != nil {
			a.log.Error("Failed to send message: %v", err)
		}
	}
}
