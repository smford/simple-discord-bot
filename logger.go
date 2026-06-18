package main

import (
	"fmt"
	"log"
	"strings"

	"github.com/spf13/viper"
)

func logger(logLevel string, format string, args ...interface{}) {

	minLogLevel := currentLogLevel

	logLevelEmoji := getLogLevelEmoji(logLevel)

	// Format the message
	message := fmt.Sprintf(format, args...)

	// Always log to the console
	log.Printf(logLevelEmoji+"%s", message)

	if shouldLog(logLevel, minLogLevel) {

		if dg == nil {
			log.Println("⚠️ Discord session is nil")
		}

		if !viper.IsSet("_log_discord_channel_id") {
			log.Printf(getLogLevelEmoji("emergency") + "No _log_discord_channel_id configured")
		} else {
			if discordConnected {
				channelID := viper.GetString("_log_discord_channel_id")
				sendEmbedMessageToDiscord(channelID, getLogLevelColor(logLevel), logLevelEmoji+strings.Title(logLevel), message)
			} else {
				log.Println("⚠️ Discord is not connected - only logging to console")
			}
		}
	}
}

// Map of log levels to their color codes
var logLevelColors = map[string]int{
	"debug":     0x85929e, // Gray
	"info":      0xaed6f1, // Light Blue
	"notice":    0x3498db, // Blue
	"warning":   0xeb984e, // Light Orange
	"error":     0xFF0000, // Red
	"critical":  0xd35400, // Dark Orange
	"alert":     0x922b21, // Dark Red
	"emergency": 0x6c3483, // Purple
	"success":   0x2ecc71, // Green
}

// getLogLevelColor takes a log level as input and returns the corresponding color.
func getLogLevelColor(level string) int {
	// Check if the log level exists in the map, otherwise default to black (0x000000).
	color, exists := logLevelColors[level]
	if !exists {
		logger("warning", "Unknown log level: %s. Defaulting to black.", level)
		return 0x000000 // Black color for unknown log levels
	}
	return color
}

// Map of log levels to their relevant emojis
var logLevelEmojis = map[string]string{
	"debug":     "🐞",  // Bug Emoji (Debug)
	"info":      "ℹ️", // Information Emoji (Info)
	"notice":    "🔔",  // Bell Emoji (Notice)
	"warning":   "⚠️", // Warning Emoji (Warning)
	"error":     "❌",  // Error Emoji (Error)
	"critical":  "🔥",  // Fire Emoji (Critical)
	"alert":     "🚨",  // Police Car Light Emoji (Alert)
	"emergency": "💀",  // Skull Emoji (Emergency)
	"success":   "✅",  // Check Mark Emoji (Success)
}

// getLogLevelEmoji takes a log level as input and returns the corresponding emoji.
func getLogLevelEmoji(level string) string {
	// Check if the log level exists in the map, otherwise return a question mark emoji.
	emoji, exists := logLevelEmojis[level]
	if !exists {
		logger("warning", "Unknown log level: %s. Defaulting to question mark emoji.", level)
		emoji = "❓" // Default emoji for unknown log levels
	}
	return emoji + " "
}

// Define a map of log levels to their numeric values.
var logLevels = map[string]int{
	"debug":     1, // Debug is level 1
	"info":      2, // Info is level 2
	"notice":    3, // Notice is level 3
	"warning":   4, // Warning is level 4
	"error":     5, // Error is level 5
	"critical":  6, // Critical is level 6
	"alert":     7, // Alert is level 7
	"emergency": 8, // Emergency is level 8
	"success":   9, // Success is level 9
}

// Function to check if a log level should be triggered based on the minimum level.
func shouldLog(currentLevel string, minLevel string) bool {
	// Get the numeric value for both the current log level and the minimum level
	currentLevelValue, currentExists := logLevels[currentLevel]
	minLevelValue, minExists := logLevels[minLevel]

	if !currentExists || !minExists {
		log.Fatal("Unknown log level provided")
	}

	// If the current level is greater than or equal to the minimum level, log it.
	return currentLevelValue >= minLevelValue
}
