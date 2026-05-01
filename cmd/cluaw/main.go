package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/sceptye/go-cluaw/internal/agent"
	"github.com/sceptye/go-cluaw/internal/config"
	"github.com/sceptye/go-cluaw/internal/discord"
	"github.com/sceptye/go-cluaw/internal/logger"
	"github.com/sceptye/go-cluaw/internal/scheduler"
	"github.com/sceptye/go-cluaw/internal/session"
	"github.com/sceptye/go-cluaw/internal/workspace"
)

func main() {
	log := logger.New()

	cfg, err := config.Load("config.yaml")
	if err != nil {
		log.Error("Failed to load config: %v", err)
		os.Exit(1)
	}

	workspacePath := cfg.ExpandWorkspace()

	ws, err := workspace.New(workspacePath)
	if err != nil {
		log.Error("Failed to load workspace: %v", err)
		os.Exit(1)
	}

	store, err := session.NewStore(workspacePath + "/sessions")
	if err != nil {
		log.Error("Failed to initialize session store: %v", err)
		os.Exit(1)
	}

	agentLoop := agent.New(cfg, ws, store, log)

	// Initialize scheduler with callback to agent
	sched := scheduler.New(cfg, log, func(message string) error {
		log.Info("Scheduler: triggering task with message: %s", message)
		resp, err := agentLoop.Process("scheduler", message)
		if err != nil {
			return err
		}
		log.Debug("Scheduler: task response: %s", resp)
		return nil
	})

	// Set scheduler on tool registry
	agentLoop.Tools().SetScheduler(sched)

	// Wire scheduler to Lua sandbox for Lua-accessible scheduling
	agentLoop.Tools().LuaBox().SetScheduler(sched)

	if err := sched.Start(); err != nil {
		log.Error("Failed to start scheduler: %v", err)
		// Continue without scheduler - not fatal
	} else {
		defer sched.Stop()
	}

	adapter, err := discord.New(cfg.Discord, agentLoop, log)
	if err != nil {
		log.Error("Failed to create Discord adapter: %v", err)
		os.Exit(1)
	}

	if err := adapter.Connect(); err != nil {
		log.Error("Failed to connect to Discord: %v", err)
		os.Exit(1)
	}
	defer adapter.Disconnect()

	// Wire Discord adapter to tool registry for message tool
	agentLoop.Tools().SetDiscord(adapter.Session())
	if channelID := cfg.Discord.DefaultChannel; channelID != "" {
		agentLoop.Tools().SetChannel(channelID)
	}

	log.Info("Cluaw is running. Press Ctrl+C to exit.")

	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM)
	<-sc
}
