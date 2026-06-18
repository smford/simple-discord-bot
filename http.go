package main

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"github.com/spf13/viper"
)

func initHTTPListener() {

	if !viper.IsSet("_http_api_port") {
		logger("info", "HTTP API port not set, not starting HTTP API server")
		return
	}

	// Set up the route and handler
	http.HandleFunc("/sendMessage/", sendMessageHandler)
	http.HandleFunc("/sendEmbedMessage/", sendEmbedMessageHandler)

	// Start the server on port 12345
	// Start the server in a goroutine so it doesn't block further code
	go func() {
		port := viper.GetString("_http_api_port")
		logger("debug", "Starting API web server on port %s...", port)
		if err := http.ListenAndServe(":"+port, nil); err != nil {
			logger("fatal", "Error starting server: %s", err)
		}
	}()

}

// Helper function to send response
func sendResponse(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	_, err := w.Write([]byte(message))
	if err != nil {
		log.Printf("Error writing response: %v", err)
	}
}

// Helper function to parse the channelID and message from the query parameters
func getChannelAndMessage(query url.Values) (string, string, error) {
	channelID := query.Get("channelID")
	message := query.Get("message")
	if channelID == "" || message == "" {
		return "", "", fmt.Errorf("missing required parameters")
	}
	return channelID, message, nil
}

// Helper function to parse the 'colour' query parameter
func getColour(query url.Values) (int, error) {
	colour := query.Get("colour")
	colourInt64, err := strconv.ParseInt(colour, 16, 0)
	if err != nil {
		return 0, fmt.Errorf("invalid colour parameter, must be an integer")
	}
	return int(colourInt64), nil
}

// General function to handle both message types
func handleMessage(w http.ResponseWriter, r *http.Request, isEmbed bool) {
	if !checkAPIRequestIsValid(w, r) {
		return
	}

	// Parse the URL query parameters
	query := r.URL.Query()

	// Extract the common parameters (channelID and message)
	channelID, message, err := getChannelAndMessage(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Retrieve the channel
	channel, _ := dg.Channel(channelID)

	// Log the action
	logger("info", "Sending message to channel %s (%s)\n%s", channel.Name, channelID, message)

	// If it's an embed message, get the additional 'colour' and 'title' parameters
	if isEmbed {
		colour, err := getColour(query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		title := query.Get("title")

		// Send the embed message to Discord
		sendEmbedMessageToDiscord(channelID, colour, title, message)
	} else {
		// Send a regular message to Discord
		sendMessageToDiscord(channelID, message)
	}

	// Send a successful response
	sendResponse(w, http.StatusOK, "OK")
}

// Handler for sending a regular message
func sendMessageHandler(w http.ResponseWriter, r *http.Request) {
	handleMessage(w, r, false)
}

// Handler for sending an embed message
func sendEmbedMessageHandler(w http.ResponseWriter, r *http.Request) {
	handleMessage(w, r, true)
}

func checkAPIRequestIsValid(w http.ResponseWriter, r *http.Request) bool {

	query := r.URL.Query()

	// Extract the parameters apiKey, channelID, and message from the URL
	apiKey := query.Get("apiKey")
	channelID := query.Get("channelID")

	if viper.IsSet("http_api_keys." + apiKey) {
		logger("debug", "API key is valid")
	} else {
		logger("error", "API key %s is invalid", apiKey)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized - Invalid API key"))
		return false
	}

	if sliceContainsValue(viper.GetStringSlice("http_api_keys."+apiKey+".channels"), channelID) {
		logger("debug", "Channel ID is valid")
	} else {
		logger("error", "Channel ID %s is invalid", channelID)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Unauthorized - Not allowed to send messages to this channel"))
		return false
	}

	return true
}
