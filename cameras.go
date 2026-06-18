package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/viper"
)

type SnapshotResponse struct {
	EventID string `json:"event_id"`
}

// custom command function to take a camera snapshot
func cameraSnapshot(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {

	words := strings.Split(content, " ")

	// get camera from message
	camera := strings.Join(words[0:1], " ")

	if camera != "" {

		// Define the API endpoint
		url := viper.GetString("_camera_api_url") + "/api/events/" + camera + "/Discord Snapshot/create"

		// Create a POST request
		req, err := http.NewRequest("POST", url, nil)
		if err != nil {
			logger("error", "Error creating request: %v", err)
			privateMessageCreate(s, m.Author.ID, fmt.Sprintf("Error creating request: %v", err), false)
		}
		req.Header.Set("Content-Type", "application/json")

		// Send the POST request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			logger("error", "Error sending POST request: %v", err)
			privateMessageCreate(s, m.Author.ID, fmt.Sprintf("Error sending POST request: %v", err), false)
		}
		defer resp.Body.Close()

		// Check if the request was successful
		if resp.StatusCode != http.StatusOK {
			logger("warning", "Request failed with status: %d", resp.StatusCode)
			privateMessageCreate(s, m.Author.ID, fmt.Sprintf("Request failed with status: %d", resp.StatusCode), false)
		} else {

			// Read the response body
			body, err := ioutil.ReadAll(resp.Body)
			if err != nil {
				logger("warning", "Error reading response body: %v", err)
				privateMessageCreate(s, m.Author.ID, fmt.Sprintf("Error reading response body: %v", err), false)
			}

			// Parse the JSON response
			var response SnapshotResponse
			err = json.Unmarshal(body, &response)
			if err != nil {
				logger("warning", "Error parsing JSON: %v", err)
				privateMessageCreate(s, m.Author.ID, fmt.Sprintf("Error parsing JSON: %v", err), false)
			}
			privateMessageCreate(s, m.Author.ID, viper.GetString("_camera_snapshot_url")+"/"+camera+"-"+response.EventID+".jpg", false)
		}
	} else {
		privateMessageCreate(s, m.Author.ID, "Camera not found", false)
	}
}

// custom command function to list cameras
func cameraList(s *discordgo.Session, m *discordgo.MessageCreate, command string, content string) {

	// Define the API endpoint
	url := viper.GetString("_camera_api_url") + "/api/config"

	// Create a GET request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		logger("error", "Error creating request: %v", err)
		privateMessageCreate(s, m.Author.ID, fmt.Sprintf("Error creating request: %v", err), false)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the GET request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		logger("error", "Error sending GET request: %v", err)
		privateMessageCreate(s, m.Author.ID, fmt.Sprintf("Error sending GET request: %v", err), false)
	}
	defer resp.Body.Close()

	// Check if the request was successful
	if resp.StatusCode != http.StatusOK {
		logger("error", "Request failed with status: %d", resp.StatusCode)
		privateMessageCreate(s, m.Author.ID, fmt.Sprintf("Request failed with status: %d", resp.StatusCode), false)
	}

	// Read the response body
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		logger("error", "Error reading response body: %v", err)
		privateMessageCreate(s, m.Author.ID, fmt.Sprintf("Error reading response body: %v", err), false)
	}

	var data map[string]interface{}

	// Parse the JSON data
	err2 := json.Unmarshal([]byte(body), &data)
	if err != nil {
		logger("emergency", "Error parsing JSON: %v", err2)
	}

	// Extract the cameras object
	cameras, ok := data["cameras"].(map[string]interface{})
	if !ok {
		logger("emergency", "Error extracting cameras data")
	}

	// Concatenate keys into a single string with newline characters
	var result string
	for key := range cameras {
		result += key + "\n"
	}

	privateMessageCreate(s, m.Author.ID, "**Camera List**\n"+result, false)

}
