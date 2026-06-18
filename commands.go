package main

import (
	"sort"
	"strconv"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

// Helper function to find a command by name or alias
func findCommandOrAlias(commandToCheck string) (string, bool) {
	// Check if it's a direct command
	if _, ok := viper.GetStringMap("commands")[commandToCheck]; ok {
		return commandToCheck, true
	}

	// Check if it's an alias
	allCommands := viper.GetStringMap("commands")
	for commandName, commandInfo := range allCommands {
		if commandData, ok := commandInfo.(map[string]interface{}); ok {
			if aliases, hasAliases := commandData["aliases"]; hasAliases {
				if aliasesList, ok := aliases.([]interface{}); ok {
					for _, alias := range aliasesList {
						if aliasStr, ok := alias.(string); ok && aliasStr == commandToCheck {
							return commandName, true
						}
					}
				}
			}
		}
	}

	return "", false
}

func findCommand(thecommand string) (string, bool, map[string]string) {

	isValidCommand := false

	allparts := strings.Split(thecommand, " ")
	num_allparts := len(allparts)

	var checkthiscommand string = ""

	var lastvalidcommandfound string = ""

	var option_num int = 0

	options := make(map[string]string)

	for i := 0; i < num_allparts; i++ {
		if i == 0 {
			checkthiscommand = allparts[0]
		} else {
			checkthiscommand = checkthiscommand + " " + allparts[i]
		}

		if canonicalCommand, ok := findCommandOrAlias(checkthiscommand); ok {
			lastvalidcommandfound = canonicalCommand
			isValidCommand = true

			// assume all remaining unparse tokens are optional.  each loop will update the list until no further valid commands are found
			option_num = 0
			new_options := make(map[string]string)
			for oi := i + 1; oi < num_allparts; oi++ {
				new_options["{"+strconv.Itoa(option_num)+"}"] = allparts[oi]
				option_num++
			}

			options = new_options

		} else {
			// command not matched, continue iterating through commands looking for the longest matching combination
		}

	}

	return lastvalidcommandfound, isValidCommand, options
}

// custom command function for sending messages as the bot
func sendMessage(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {

	// split the string by whitespace
	words := strings.Split(content, " ")

	// get channel ID
	channelID := strings.Join(words[0:1], " ")

	// Get the last words ignoring the first
	message := strings.Join(words[1:], " ")

	// send message to channel
	s.ChannelMessageSend(channelID, message)
}

// custom command function for editing messages as the bot
func editMessage(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {

	// split the string by whitespace
	words := strings.Split(content, " ")

	// get channel ID
	channelID := strings.Join(words[0:1], " ")

	// get message ID
	messageID := strings.Join(words[1:2], " ")

	// Get the last words ignoring the first two
	message := strings.Join(words[2:], " ")

	// edits message in channel
	s.ChannelMessageEdit(channelID, messageID, message)
}

// custom command function to list all Emoji
func listEmoji(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {

	words := strings.Split(content, " ")

	// get guild ID from message
	guildID := strings.Join(words[0:1], " ")

	//	var guildID string = m.GuildID

	if guildID == "" {
		guildID = m.GuildID
	}

	if guildID != "" {
		emojis, err := s.GuildEmojis(guildID)
		if err != nil {
			logger("error", "Could not get emoji with error: %s", err)
		}

		var message string

		for _, emoji := range emojis {
			if m.GuildID != "" {
				message += "<:" + emoji.Name + ":" + emoji.ID + ">  `" + emoji.ID + "    " + emoji.Name + "`\n"
			} else {
				message += emoji.ID + "    " + emoji.Name + "\n"
			}
		}

		if m.GuildID != "" {
			channelMessageCreate(s, m, "**Emoji for "+guildID+"**\n"+message, false)
		} else {
			privateMessageCreate(s, m.Author.ID, "**Emoji for "+guildID+"**\n```"+message+"```", false)
		}
	} else {
		if m.GuildID != "" {
			channelMessageCreate(s, m, "Guild/Server ID not found", false)
		} else {
			privateMessageCreate(s, m.Author.ID, "Guild/Server ID not found", false)
		}
	}
}

// custom command function to list all commands based on user permission
func showHelp(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {

	user, _ := s.GuildMember(viper.GetString("_discord_default_server_id"), m.Author.ID)

	var helpCommands = make(map[string]string)
	var helpMessage string
	var longestCommandLength int = 0

	commandkey := viper.GetString("_command_key")

	allCommands := viper.GetStringMap("commands")

	// Loop through the commands map
	for command, info := range allCommands {

		// check if user has permission to execute a command
		var canRun bool = false

		// Access the "roles" for each command
		roles, ok := info.(map[string]interface{})["roles"]
		if !ok {
			logger("warning", "Help information not found for command %s", command)
			continue
		}

		for _, role := range roles.([]interface{}) {
			if checkUserPerms(role.(string), user, m.Author.ID) {
				canRun = true
			}
		}

		if canRun {
			// Access the "help" field for each command
			help, ok := info.(map[string]interface{})["help"].(string)
			if !ok {
				logger("warning", "Help information not found for command %s", command)
				continue
			}

			// Add the main command
			mainCommand := commandkey + " " + command
			if len(mainCommand) > longestCommandLength {
				longestCommandLength = len(mainCommand)
			}
			helpCommands[mainCommand] = help

			// Add aliases as separate commands
			if aliases, hasAliases := info.(map[string]interface{})["aliases"]; hasAliases {
				if aliasesList, ok := aliases.([]interface{}); ok {
					for _, alias := range aliasesList {
						if aliasStr, ok := alias.(string); ok {
							aliasCommand := commandkey + " " + aliasStr
							if len(aliasCommand) > longestCommandLength {
								longestCommandLength = len(aliasCommand)
							}
							helpCommands[aliasCommand] = help
						}
					}
				}
			}
		}

	}

	// Sort the commands alphabetically
	keys := make([]string, 0, len(helpCommands))
	for key := range helpCommands {
		keys = append(keys, key)
	}

	// Sort the keys
	sort.Strings(keys)

	// Iterate over the sorted keys and access the map values
	for _, key := range keys {
		helpMessage += key + strings.Repeat(" ", longestCommandLength+2-len(key)) + "- " + helpCommands[key] + "\n"
	}

	helpMessage = "Help Commands:\n--------------\n" + helpMessage

	privateMessageCreate(s, m.Author.ID, helpMessage, true, true)
}

// custom command function for showing the version
func showVersion(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	messagetosend := "**__Simple Discord Bot__**\n**Version:** " + applicationVersion + "\n**Built:** " + buildDateTime
	channelMessageCreate(s, m, messagetosend, false)
}

// custom command function for changing the current log level
func logLevelSet(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	var messagetosend string
	if _, exists := logLevels[content]; exists {
		currentLogLevel = content

		viper.Set("_log_level", currentLogLevel)
		viper.WriteConfig()

		messagetosend = "Log level successfully updated to **" + currentLogLevel + "**"
	} else {
		messagetosend = "**" + command + "** is not a valid log level it must be one of debug, info, notice, warning, error, critical, alert, emergency"
	}

	channelMessageCreate(s, m, messagetosend, false)
}

// custom command function for showing the version
func logLevelShow(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {
	messagetosend := "**Current Log Level**: " + currentLogLevel
	channelMessageCreate(s, m, messagetosend, false)
}
