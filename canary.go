package main

import (
	"net/http"
	"time"
)

func canaryCheckin(url string, interval int) {
	// First immediate check-in
	performCheckin(url)

	// Start periodic check-ins
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		performCheckin(url)
	}
}

func performCheckin(url string) {
	client := http.Client{
		Timeout: 5 * time.Second,
	}
	resp, err := client.Get(url)
	if err != nil {
		logger("error", "Could not connect to canary with error: %s", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		logger("error", "Unexpected response status: %d", resp.StatusCode)
	} else {
		logger("info", "Canary Checkin successful")
	}
}
