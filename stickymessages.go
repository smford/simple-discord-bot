package main

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

// stickyMessageTimers tracks active timers for each channel
var stickyMessageTimers map[string]*time.Timer = make(map[string]*time.Timer)

// handleStickyMessage manages sticky messages for configured channels
func handleStickyMessage(s *discordgo.Session, m *discordgo.MessageCreate) {
	channelID := m.ChannelID

	// Check if this channel is configured for sticky messages
	stickyChannels := viper.GetStringMap("sticky_messages.channels")
	if stickyChannels == nil {
		return
	}

	// Check if this channel ID exists in the sticky messages config
	_, exists := stickyChannels[channelID]
	if !exists {
		return
	}

	logger("debug", "Handling sticky message for channel %s", channelID)

	// Cancel any existing timer for this channel
	if timer, exists := stickyMessageTimers[channelID]; exists {
		timer.Stop()
		delete(stickyMessageTimers, channelID)
		logger("debug", "Cancelled existing sticky message timer for channel %s", channelID)
	}

	// Get the delay from config (default to 10 seconds)
	delaySeconds := viper.GetInt("sticky_messages.delay_seconds")
	if delaySeconds == 0 {
		delaySeconds = 10
	}

	// Create a new timer
	stickyMessageTimers[channelID] = time.AfterFunc(time.Duration(delaySeconds)*time.Second, func() {
		updateStickyMessage(s, channelID)
		// Clean up the timer from the map
		delete(stickyMessageTimers, channelID)
	})

	logger("debug", "Set sticky message timer for channel %s with delay of %d seconds", channelID, delaySeconds)
}

// updateStickyMessage updates the sticky message for a channel
func updateStickyMessage(s *discordgo.Session, channelID string) {
	logger("debug", "Updating sticky message for channel %s", channelID)

	// Get the current sticky message ID from config
	currentStickyMessageID := viper.GetString(fmt.Sprintf("sticky_messages.channels.%s.old_message_id", channelID))

	// Get the configured message content for this channel
	configuredMessage := viper.GetString(fmt.Sprintf("sticky_messages.channels.%s.message", channelID))

	if configuredMessage == "" {
		logger("debug", "No configured message content for channel %s", channelID)
		return
	}

	// Delete the current sticky message if it exists
	if currentStickyMessageID != "" && currentStickyMessageID != "0" {
		err := s.ChannelMessageDelete(channelID, currentStickyMessageID)
		if err != nil {
			logger("warning", "Failed to delete old sticky message %s in channel %s: %s", currentStickyMessageID, channelID, err)
		} else {
			logger("debug", "Deleted old sticky message %s in channel %s", currentStickyMessageID, channelID)
		}
	}

	// Post the new sticky message using the configured content
	newStickyMessage, err := s.ChannelMessageSend(channelID, configuredMessage)
	if err != nil {
		logger("error", "Failed to send new sticky message to channel %s: %s", channelID, err)
		return
	}

	logger("info", "Created new sticky message %s for channel %s", newStickyMessage.ID, channelID)

	// Update the config with the new sticky message ID
	updateStickyMessageConfig(channelID, configuredMessage, newStickyMessage.ID)
}

// updateStickyMessageConfig updates the sticky message ID and content in the configuration
func updateStickyMessageConfig(channelID, messageContent, messageID string) {
	// Set the message content and old message ID for this channel
	viper.Set(fmt.Sprintf("sticky_messages.channels.%s.message", channelID), messageContent)
	viper.Set(fmt.Sprintf("sticky_messages.channels.%s.old_message_id", channelID), messageID)

	// Write the config file
	err := viper.WriteConfig()
	if err != nil {
		logger("error", "Failed to update sticky message config for channel %s: %s", channelID, err)
		return
	}

	logger("debug", "Updated sticky message config: channel %s -> message %s, ID %s", channelID, messageContent, messageID)
}

// initStickyMessages sends sticky messages for channels that don't have an old_message_id on startup
func initStickyMessages(s *discordgo.Session) {
	logger("info", "Initializing sticky messages...")

	// Get sticky messages configuration
	stickyChannels := viper.GetStringMap("sticky_messages.channels")
	if stickyChannels == nil {
		logger("debug", "No sticky messages configured")
		return
	}

	// Check each configured channel
	for channelID := range stickyChannels {
		// Get the configured message content
		configuredMessage := viper.GetString(fmt.Sprintf("sticky_messages.channels.%s.message", channelID))
		oldMessageID := viper.GetString(fmt.Sprintf("sticky_messages.channels.%s.old_message_id", channelID))

		// Skip if no message is configured
		if configuredMessage == "" {
			logger("debug", "No message configured for channel %s, skipping", channelID)
			continue
		}

		// Send sticky message if no old_message_id exists or it's empty/"0"
		if oldMessageID == "" || oldMessageID == "0" {
			logger("info", "Sending initial sticky message for channel %s", channelID)

			// Post the sticky message
			newStickyMessage, err := s.ChannelMessageSend(channelID, configuredMessage)
			if err != nil {
				logger("error", "Failed to send initial sticky message to channel %s: %s", channelID, err)
				continue
			}

			logger("success", "Sent initial sticky message %s for channel %s", newStickyMessage.ID, channelID)

			// Update the config with the new message ID
			updateStickyMessageConfig(channelID, configuredMessage, newStickyMessage.ID)
		} else {
			logger("debug", "Channel %s already has sticky message %s, checking if it's still the latest", channelID, oldMessageID)

			// Check if the latest message is still the sticky message
			messages, err := s.ChannelMessages(channelID, 1, "", "", "")
			if err != nil {
				logger("error", "Failed to get latest message for channel %s: %s", channelID, err)
				continue
			}

			// If there are no messages, post the sticky message
			if len(messages) == 0 {
				logger("info", "No messages in channel %s, posting sticky message", channelID)
				newStickyMessage, err := s.ChannelMessageSend(channelID, configuredMessage)
				if err != nil {
					logger("error", "Failed to send sticky message to empty channel %s: %s", channelID, err)
					continue
				}
				logger("success", "Posted sticky message %s to empty channel %s", newStickyMessage.ID, channelID)
				updateStickyMessageConfig(channelID, configuredMessage, newStickyMessage.ID)
				continue
			}

			latestMessage := messages[0]

			// If the latest message is not the sticky message, refresh it
			if latestMessage.ID != oldMessageID {
				logger("info", "Latest message in channel %s is not the sticky message, refreshing", channelID)

				// Delete the old sticky message
				err = s.ChannelMessageDelete(channelID, oldMessageID)
				if err != nil {
					logger("warning", "Failed to delete old sticky message %s in channel %s: %s", oldMessageID, channelID, err)
				} else {
					logger("debug", "Deleted old sticky message %s in channel %s", oldMessageID, channelID)
				}

				// Post the new sticky message
				newStickyMessage, err := s.ChannelMessageSend(channelID, configuredMessage)
				if err != nil {
					logger("error", "Failed to refresh sticky message in channel %s: %s", channelID, err)
					continue
				}

				logger("success", "Refreshed sticky message %s for channel %s", newStickyMessage.ID, channelID)
				updateStickyMessageConfig(channelID, configuredMessage, newStickyMessage.ID)
			} else {
				logger("debug", "Sticky message %s is still the latest in channel %s", oldMessageID, channelID)
			}
		}
	}

	logger("info", "Sticky messages initialization complete")
}
