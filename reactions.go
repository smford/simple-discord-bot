package main

import (
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

// discord addReaction handler
func addReaction(s *discordgo.Session, mr *discordgo.MessageReactionAdd) {
	for _, v := range viper.GetStringMap("discord_reactions") {
		if m, ok := v.(map[string]interface{}); ok {
			// check message id is being tracked
			if strconv.Itoa(m["message_id"].(int)) == mr.MessageID {

				// check emoji is being tracked for this message
				emoji := strings.Split(m["emoji"].(string), ":")
				if emoji[0] == mr.Emoji.Name {
					// check which type of reaction this is
					if m["type"] == "role" {
						// add role
						s.GuildMemberRoleAdd(mr.GuildID, mr.UserID, strconv.Itoa(m["role_id"].(int)))
					}
				}
			}
		} else {
			logger("warning", "Data is not a map[string]interface{}")
		}
	}
}

// discord removeReaction handler
func removeReaction(s *discordgo.Session, mr *discordgo.MessageReactionRemove) {
	for _, v := range viper.GetStringMap("discord_reactions") {
		if m, ok := v.(map[string]interface{}); ok {
			// check message id is being tracked
			if strconv.Itoa(m["message_id"].(int)) == mr.MessageID {

				// check emoji is being tracked for this message
				emoji := strings.Split(m["emoji"].(string), ":")
				if emoji[0] == mr.Emoji.Name {
					// check which type of reaction this is
					if m["type"] == "role" {
						// remove role
						s.GuildMemberRoleRemove(mr.GuildID, mr.UserID, strconv.Itoa(m["role_id"].(int)))
					}
				}
			}
		} else {
			logger("warning", "Data is not a map[string]interface{}")
		}
	}
}

// check reactions
func checkReactions(s *discordgo.Session) {
	logger("info", "Checking reactions for tracked messages")
	for _, v := range viper.GetStringMap("discord_reactions") {
		if m, ok := v.(map[string]interface{}); ok {
			channelID := strconv.Itoa(m["channel_id"].(int))
			messageID := strconv.Itoa(m["message_id"].(int))

			// check emoji is being tracked for this message
			messageReactions, err := s.MessageReactions(channelID, messageID, m["emoji"].(string), 100, "", "")
			if err != nil {
				logger("error", "Checking reactions channelID: %s messageID: %s, Error: %s", channelID, messageID, err)
			}
			var hasBotReaction bool = false
			for _, user := range messageReactions {
				if user.ID == s.State.User.ID {
					hasBotReaction = true
				}
			}

			if !hasBotReaction {
				s.MessageReactionAdd(channelID, messageID, m["emoji"].(string))
				// pause to make sure reactions are added in order
				time.Sleep(1 * time.Second)
			}

		}
	}
}
