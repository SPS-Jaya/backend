package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type Traveler struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type AirportInfo struct {
	Airport string `json:"airport"`
	Date    string `json:"date"`
	Time    string `json:"time"`
}

type ItineraryRequest struct {
	Countries []string    `json:"countries"`
	Arrival   AirportInfo `json:"arrival"`
	Departure AirportInfo `json:"departure"`
	Travelers []Traveler  `json:"travelers"`
}

func main() {
	// Create the request body
	requestBody := ItineraryRequest{
		Countries: []string{"Switzerland", "France"},
		Arrival: AirportInfo{
			Airport: "ZRH",
			Date:    "2025-07-01",
			Time:    "09:00",
		},
		Departure: AirportInfo{
			Airport: "CDG",
			Date:    "2025-07-11",
			Time:    "18:00",
		},
		Travelers: []Traveler{
			{Name: "Alice", Age: 30},
			{Name: "Bob", Age: 32},
			{Name: "Charlie", Age: 12},
		},
	}

	// Convert request body to JSON
	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		fmt.Printf("Error marshaling JSON: %v\n", err)
		return
	}

	// Create HTTP request
	req, err := http.NewRequest("POST", "https://gsc2025-sps-418414887688.us-central1.run.app/run", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")

	// Create HTTP client and send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	// Print response
	fmt.Printf("Response Status: %s\n", resp.Status)
	fmt.Printf("Response Body: %s\n", string(body))
}
