package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"
)

// ResolvedDetails contains the scraped flight information
type ResolvedDetails struct {
	Destination     string `json:"destination"`
	RealDestination string `json:"real_destination"`
	Model           string `json:"model"`
	Origin          string `json:"origin"`
}

// Scraper handles fetching data from external websites
type Scraper struct {
	client *http.Client
}

func NewScraper() *Scraper {
	return &Scraper{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// FetchFlightDetails scrapes FlightAware for destination and model info
func (s *Scraper) FetchFlightDetails(callsign string) (*ResolvedDetails, error) {
	url := fmt.Sprintf("https://www.flightaware.com/live/flight/%s", callsign)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Mimic headers to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	// Regex to find trackpollBootstrap JSON object
	// matches: trackpollBootstrap = { ... };
	re := regexp.MustCompile(`(?:var\s+)?trackpollBootstrap\s*=\s*({.+?});`)
	matches := re.FindStringSubmatch(bodyStr)
	if len(matches) < 2 {
		return nil, fmt.Errorf("no data found in page")
	}

	jsonStr := matches[1]
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return nil, fmt.Errorf("failed to parse json: %v", err)
	}

	flightsData, ok := data["flights"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("no flights data structure")
	}

	// Iterate over flight IDs (keys are opaque strings)
	for _, flightData := range flightsData {
		fd, ok := flightData.(map[string]interface{})
		if !ok {
			continue
		}
		actLog, ok := fd["activityLog"].(map[string]interface{})
		if !ok {
			continue
		}
		fLog, ok := actLog["flights"].([]interface{})
		if !ok || len(fLog) == 0 {
			continue
		}

		// Use the first (latest) flight log entry
		latest, ok := fLog[0].(map[string]interface{})
		if !ok {
			continue
		}

		destData, _ := latest["destination"].(map[string]interface{})
		aircraftData, _ := latest["aircraft"].(map[string]interface{})
		originData, _ := latest["origin"].(map[string]interface{})

		destName := ""
		if v, ok := destData["friendlyLocation"].(string); ok {
			destName = v
		} else if v, ok := destData["iata"].(string); ok {
			destName = v
		}

		model := ""
		if v, ok := aircraftData["friendlyType"].(string); ok {
			model = v
		} else if v, ok := aircraftData["type"].(string); ok {
			model = v
		}

		originName := ""
		if v, ok := originData["friendlyLocation"].(string); ok {
			originName = v
		}

		// Return raw data, game logic handled in main.go
		return &ResolvedDetails{
			Destination:     destName, // Just use the real destination here
			RealDestination: destName,
			Model:           model,
			Origin:          originName,
		}, nil
	}

	return nil, fmt.Errorf("details not found in flight data")
}
