package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type Flight struct {
	Icao24      string  `json:"icao24"`
	Callsign    string  `json:"callsign"`
	Lon         float64 `json:"lon"`
	Lat         float64 `json:"lat"`
	VelocityKts int     `json:"velocity_kts"`
	Heading     float64 `json:"heading"`
	AltitudeFt  int     `json:"altitude_ft"`
	OnGround    bool    `json:"on_ground"`
	Origin      string  `json:"origin_country"`
	Category    string  `json:"category"`
	Destination string  `json:"destination"` // Inferred
}

const (
	openSkyURL      = "https://opensky-network.org/api/states/all"
	openSkyAuthURL  = "https://auth.opensky-network.org/auth/realms/opensky-network/protocol/openid-connect/token"
	cacheDuration   = 10 * time.Second
	credentialsPath = "./credentials.json"
)

var categoryMap = map[int]string{
	0: "No Info", 1: "No Info", 2: "Light", 3: "Small",
	4: "Large", 5: "High Vortex", 6: "Heavy", 7: "High Perf",
	8: "Rotorcraft", 9: "Glider", 10: "Lighter-than-air",
	11: "Parachutist", 12: "Ultralight", 13: "Reserved",
	14: "UAV", 15: "Space", 16: "Emergency", 17: "Service",
	18: "Point Obstacle", 19: "Cluster", 20: "Line Obstacle",
}

type FlightClient struct {
	httpClient *http.Client
	cache      []Flight
	lastFetch  time.Time
	mu         sync.Mutex
	token      string
	clientID   string
	clientSec  string
}

func NewFlightClient() *FlightClient {
	fc := &FlightClient{
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
	fc.loadCredentials()
	return fc
}

func (fc *FlightClient) loadCredentials() {
	// Try Env vars first
	id := os.Getenv("CLIENT_ID")
	secret := os.Getenv("CLIENT_SECRET")

	if id != "" && secret != "" {
		fc.clientID = id
		fc.clientSec = secret
		return
	}

	fmt.Println("CLIENT_ID from env:", fc.clientID)

	// Try loading from file
	if _, err := os.Stat(credentialsPath); err == nil {
		// Simple JSON parsing if file exists
		file, err := os.Open(credentialsPath)
		if err == nil {
			defer file.Close()
			var creds struct {
				ClientID     string `json:"clientId"`
				ClientSecret string `json:"clientSecret"`
			}
			if err := json.NewDecoder(file).Decode(&creds); err == nil {
				fc.clientID = creds.ClientID
				fc.clientSec = creds.ClientSecret
			}
		}
	}

	fmt.Println("CLIENT_ID from file:", fc.clientID)
}

func (fc *FlightClient) authenticate() error {
	if fc.clientID == "" || fc.clientSec == "" {
		return nil // No credentials, use anonymous
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	data.Set("client_id", fc.clientID)
	data.Set("client_secret", fc.clientSec)

	req, err := http.NewRequest("POST", openSkyAuthURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("auth failed: %d", resp.StatusCode)
	}

	var result struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	fc.token = result.AccessToken
	return nil
}

func (fc *FlightClient) FetchFlights(centerLat, centerLon, radiusDeg float64) ([]Flight, error) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Return cached if fresh
	if time.Since(fc.lastFetch) < cacheDuration && len(fc.cache) > 0 {
		return fc.cache, nil
	}

	// Authenticate if needed (simple check: if we have creds but no token)
	if fc.clientID != "" && fc.token == "" {
		if err := fc.authenticate(); err != nil {
			fmt.Println("Warning: Authentication failed, falling back to anonymous:", err)
		}
	}

	lamin := centerLat - radiusDeg
	lamax := centerLat + radiusDeg
	lomin := centerLon - radiusDeg
	lomax := centerLon + radiusDeg

	apiURL := fmt.Sprintf("%s?lamin=%f&lomin=%f&lamax=%f&lomax=%f",
		openSkyURL, lamin, lomin, lamax, lomax)

	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}

	if fc.token != "" {
		req.Header.Set("Authorization", "Bearer "+fc.token)
	}

	resp, err := fc.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rate limit exceeded (429)")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Time   int             `json:"time"`
		States [][]interface{} `json:"states"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var flights []Flight
	for _, s := range result.States {
		// s[5] is lon, s[6] is lat. If nil, skip.
		if s[5] == nil || s[6] == nil {
			continue
		}

		lon := s[5].(float64)
		lat := s[6].(float64)
		callsign := strings.TrimSpace(s[1].(string))
		if callsign == "" {
			callsign = "N/A"
		}

		// Altitude
		altM := 0.0
		if s[7] != nil {
			altM = s[7].(float64)
		}
		altFt := int(altM * 3.28084)

		// Velocity
		velMs := 0.0
		if s[9] != nil {
			velMs = s[9].(float64)
		}
		velKts := int(velMs * 1.94384)

		// Heading
		heading := 0.0
		if s[10] != nil {
			heading = s[10].(float64)
		}

		// Category
		catStr := "Unknown"
		if len(s) > 17 && s[17] != nil {
			catCode := int(s[17].(float64))
			if val, ok := categoryMap[catCode]; ok {
				catStr = val
			}
		}

		f := Flight{
			Icao24:      s[0].(string),
			Callsign:    callsign,
			Lon:         lon,
			Lat:         lat,
			VelocityKts: velKts,
			Heading:     heading,
			AltitudeFt:  altFt,
			OnGround:    s[8].(bool),
			Origin:      s[2].(string),
			Category:    catStr,
			// Destination: inferDestination(heading), // Removed
		}
		flights = append(flights, f)
	}

	fc.cache = flights
	fc.lastFetch = time.Now()

	return flights, nil
}
