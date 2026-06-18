package main

import (
	"fmt"
	"sort"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

// loads configuration
func loadConfig(reload ...bool) {
	var isReload bool
	if len(reload) > 0 {
		isReload = reload[0]
	} else {
		isReload = false
	}
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			logger("emergency", "Config file not found")
		} else {
			logger("emergency", "Config file was found but another error was discovered: %s", err)
		}
	}

	if isReload {
		initTasks()
		initScheduledMessages()
		initRSSFeeds()
	}
}

func loadConfigCommand(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	loadConfig(true)
	logger("warning", "Configuration has been reloaded")
	privateMessageCreate(s, m.Author.ID, "Configuration has been reloaded", false)
}

// displays configuration
func displayConfig() {
	allmysettings := viper.AllSettings()
	var keys []string
	for k := range allmysettings {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		fmt.Println("CONFIG:", k, ":", allmysettings[k])
	}
}

// displays help information
func displayHelp() {
	message := `
      --config string       Configuration file: /path/to/file.yaml (default "./config.yaml")
      --displayconfig       Display configuration
      --help                Display help
      --version             Display version
`

	fmt.Println("simple-discord-bot " + applicationVersion)
	fmt.Println(message)
}
