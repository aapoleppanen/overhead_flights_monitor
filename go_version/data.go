package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Helper to get persistent file path
func (dm *DataManager) getFilePath(filename string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filename // Fallback to current dir
	}
	// Store data in a persistent subfolder hidden in home
	dir := filepath.Join(home, ".flight-monitor-data")

	// Create dir if not exists (check error to be safe, but ignore if exists)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		_ = os.MkdirAll(dir, 0755)
	}
	return filepath.Join(dir, filename)
}

const (
	scoresFile   = "scores.json"
	usersFile    = "users.json"
	airportsFile = "airports.json"
)

// UserStats represents a player's statistics
type UserStats struct {
	Name               string `json:"name"`
	GamesPlayed        int    `json:"games_played"`
	TotalScore         int    `json:"total_score"`
	BestScore          int    `json:"best_score"`
	PerformancePercent int    `json:"performance_percent,omitempty"`
}

// ScoreEntry represents a single high score entry
type ScoreEntry struct {
	Name  string `json:"name"`
	Score int    `json:"score"`
	Date  string `json:"date"` // stored as string for simplicity, matching Python version
}

// DataManager handles persistence for users and scores
type DataManager struct {
	mu sync.Mutex
}

var globalDataManager = &DataManager{}

// LoadUsers reads the users.json file and returns a map of users
func (dm *DataManager) LoadUsers() (map[string]UserStats, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	users := make(map[string]UserStats)
	data, err := os.ReadFile(dm.getFilePath(usersFile))
	if err != nil {
		if os.IsNotExist(err) {
			return users, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// SaveUser updates or creates a user's stats
func (dm *DataManager) SaveUser(name string, score int) (UserStats, error) {
	// Load existing first to ensure we have latest state
	users, err := dm.LoadUsers()
	if err != nil {
		return UserStats{}, err
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	user, ok := users[name]
	if !ok {
		user = UserStats{Name: name}
	}

	user.GamesPlayed++
	user.TotalScore += score
	if score > user.BestScore {
		user.BestScore = score
	}

	users[name] = user

	data, err := json.MarshalIndent(users, "", "  ")
	if err != nil {
		return user, err
	}

	if err := os.WriteFile(dm.getFilePath(usersFile), data, 0644); err != nil {
		return user, err
	}

	return user, nil
}

// DeleteUser removes a user from the users.json file
func (dm *DataManager) DeleteUser(name string) error {
	// Note: Calling LoadUsers() here would deadlock if we held lock,
	// but LoadUsers acquires its own lock.
	// However, we shouldn't call public methods that lock from other methods.
	// Refactoring to just load raw here for safety or trust the flow.
	// Actually, LoadUsers is fine as long as we don't hold lock across it.

	users, err := dm.LoadUsers()
	if err != nil {
		return err
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	if _, ok := users[name]; ok {
		delete(users, name)

		data, err := json.MarshalIndent(users, "", "  ")
		if err != nil {
			return err
		}

		if err := os.WriteFile(dm.getFilePath(usersFile), data, 0644); err != nil {
			return err
		}
	}

	return nil
}

// LoadScores reads the scores.json file
func (dm *DataManager) LoadScores() ([]ScoreEntry, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	var scores []ScoreEntry
	data, err := os.ReadFile(dm.getFilePath(scoresFile))
	if err != nil {
		if os.IsNotExist(err) {
			return scores, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, &scores); err != nil {
		return nil, err
	}
	return scores, nil
}

// AddScore adds a new score and keeps only top 10
func (dm *DataManager) AddScore(entry ScoreEntry) ([]ScoreEntry, error) {
	scores, err := dm.LoadScores()
	if err != nil {
		return nil, err
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	scores = append(scores, entry)

	// Sort descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Score > scores[j].Score
	})

	// Keep top 10
	if len(scores) > 10 {
		scores = scores[:10]
	}

	data, err := json.MarshalIndent(scores, "", "  ")
	if err != nil {
		return nil, err
	}

	if err := os.WriteFile(dm.getFilePath(scoresFile), data, 0644); err != nil {
		return nil, err
	}

	return scores, nil
}

// GetLeaderboard returns high scores and user stats for display
func (dm *DataManager) GetLeaderboard() ([]ScoreEntry, []UserStats, error) {
	scores, err := dm.LoadScores()
	if err != nil {
		return nil, nil, err
	}

	usersMap, err := dm.LoadUsers()
	if err != nil {
		return nil, nil, err
	}

	var userStatsList []UserStats
	for _, u := range usersMap {
		// Calculate performance
		percentage := 0
		if u.GamesPlayed > 0 {
			// Max potential score roughly 1000 per game (200 * 5 rounds)
			// Matches Python logic: percentage = int((total / (games * 1000)) * 100)
			percentage = int((float64(u.TotalScore) / float64(u.GamesPlayed*1000)) * 100)
			if percentage > 100 {
				percentage = 100
			} else if percentage < 0 {
				percentage = 0
			}
		}
		u.PerformancePercent = percentage
		userStatsList = append(userStatsList, u)
	}

	// Sort users by best score desc
	sort.Slice(userStatsList, func(i, j int) bool {
		return userStatsList[i].BestScore > userStatsList[j].BestScore
	})

	return scores, userStatsList, nil
}

// LoadAirports reads the airports.json file
func (dm *DataManager) LoadAirports() ([]string, error) {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	var airports []string
	data, err := os.ReadFile(dm.getFilePath(airportsFile))
	if err != nil {
		if os.IsNotExist(err) {
			return airports, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, &airports); err != nil {
		return nil, err
	}
	return airports, nil
}

// SaveAirport adds a new airport to the list if not present
func (dm *DataManager) SaveAirport(city string) error {
	if city == "" || city == "Unknown" || city == "N/A" {
		return nil
	}

	// Load existing without lock first to avoid deadlock with SaveAirport calling LoadAirports
	// Actually, LoadAirports uses lock. We should just call a helper or duplicate logic.
	// Or just do it all inside SaveAirport.

	// Better: We just load, check, modify, save.
	dm.mu.Lock()
	defer dm.mu.Unlock()

	var airports []string
	data, err := os.ReadFile(dm.getFilePath(airportsFile))
	if err == nil {
		json.Unmarshal(data, &airports)
	}

	// Check exists
	exists := false
	for _, a := range airports {
		if a == city {
			exists = true
			break
		}
	}

	if !exists {
		airports = append(airports, city)
		sort.Strings(airports)

		newData, err := json.MarshalIndent(airports, "", "  ")
		if err != nil {
			return err
		}
		return os.WriteFile(dm.getFilePath(airportsFile), newData, 0644)
	}

	return nil
}
