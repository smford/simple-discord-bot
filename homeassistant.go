package main

import (
	"bytes"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

// custom command function to call the Home Assistant API
func apiHomeAssistant(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {

	channelID := m.Message.ChannelID

	if viper.IsSet("commands." + command + ".channels." + channelID) {
		parameters := viper.GetStringMap("commands." + command)["channels"].(map[string]interface{})[channelID].(map[string]interface{})["parameters"]

		parameterList := parameters.([]interface{})

		for _, param := range parameterList {
			makeHomeAssistantAPIRequest(param.(string))
		}

	}
}

// make Home Assistant API request
func makeHomeAssistantAPIRequest(param string) {
	url := viper.GetString("_home_assistant_url")

	// Check if the url ends with "/"
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}

	// Check if the param starts with "/" and remove
	param = strings.TrimPrefix(param, "/")

	token := viper.GetString("_home_assistant_token")

	// JSON payload for calling the script (if needed)
	payload := []byte(`{}`)

	// Create a new HTTP request
	req, err := http.NewRequest("POST", url+param, ioutil.NopCloser(bytes.NewBuffer(payload)))
	if err != nil {
		logger("error", "Error creating request: %s", err)
		return
	}

	// Set the required headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger("error", "Error sending request: %s", err)
		return
	}
	defer resp.Body.Close()

	// Read and print the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger("error", "Error reading response: %s", err)
		return
	}

	logger("info", "Response: %s", string(body))

}
