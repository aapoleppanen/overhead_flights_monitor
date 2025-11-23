package main

import (
	"fmt"
	"image/color"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"github.com/hajimehoshi/ebiten/v2/text"
	"golang.org/x/image/font/basicfont"
)

const (
	// Physical screen dimensions (Portrait)
	physicalWidth  = 720
	physicalHeight = 1280

	// Game logic dimensions (Landscape)
	logicalWidth  = 1280
	logicalHeight = 720

	defaultZoom = 10

	// UI Colors
	colBgDark     = 0x0f172aff // #0f172a
	colAccent     = 0x38bdf8ff // #38bdf8
	colGlass      = 0x0f172af2 // #0f172a (95% opacity)
	colGlassLight = 0x334155ff // #334155 (lighter, more opaque)
	colText       = 0xf1f5f9ff // #f1f5f9
	colTextMuted  = 0x94a3b8ff // #94a3b8
	colSuccess    = 0x4ade80ff // #4ade80
	colDanger     = 0xf87171ff // #f87171
)

var (
	myLat = 60.25881233034921
	myLon = 24.780103286993022
)

// GameState enum
type State int

const (
	StateLogin State = iota
	StateMap
	StateGameBriefing
	StateGamePlaying
	StateRoundSetup // New state for fetching details
	StateGameOver
	StateLeaderboard
)

type Game struct {
	flightClient *FlightClient
	tileLoader   *TileLoader
	dataManager  *DataManager
	scraper      *Scraper
	flights      []Flight
	state        State
	shouldQuit   bool

	// Offscreen buffer for rotation
	offscreen *ebiten.Image

	// Data
	currentUser   UserStats
	usersMap      map[string]UserStats
	highScores    []ScoreEntry
	userStatsList []UserStats
	airports      []string

	// Login Input
	inputText         string
	userToDelete      string
	showDeleteConfirm bool

	// Camera
	camLat  float64
	camLon  float64
	camZoom int

	// Touch/Input
	isDragging    bool
	dragStartX    int
	dragStartY    int
	startCamLat   float64
	startCamLon   float64
	lastPinchDist float64

	// Assets
	planeImg *ebiten.Image

	// Selected Plane
	selectedPlane   *Flight
	resolvedDetails *ResolvedDetails
	resolving       bool

	// Game Logic
	score           int
	targetPlane     *Flight
	round           int
	roundStartTime  time.Time
	questionText    string // Dynamic question
	options         []string
	correctOption   string
	wrongGuess      string // Store the wrong guess for red feedback
	showResult      bool
	resultCorrect   bool
	resultStartTime time.Time

	// UI Elements (Simple rects for click detection)
	buttons []Button
}

type Button struct {
	X, Y, W, H int
	Text       string
	Action     func()
	Color      color.Color
	TextColor  color.Color
}

func NewGame(fc *FlightClient) *Game {
	g := &Game{
		flightClient: fc,
		tileLoader:   NewTileLoader(),
		dataManager:  &DataManager{},
		scraper:      NewScraper(),
		camLat:       myLat,
		camLon:       myLon,
		camZoom:      defaultZoom,
		planeImg:     createPlaneImage(),
		state:        StateLogin,
		offscreen:    ebiten.NewImage(logicalWidth, logicalHeight),
	}

	// Load initial data
	g.refreshUsers()
	g.refreshAirports()
	go g.refreshFlights()

	return g
}

func (g *Game) refreshUsers() {
	users, err := g.dataManager.LoadUsers()
	if err == nil {
		g.usersMap = users
	}
}

func (g *Game) refreshAirports() {
	airports, err := g.dataManager.LoadAirports()
	if err == nil && len(airports) > 0 {
		g.airports = airports
	} else {
		// Fallback if load failed or file empty
		g.airports = []string{"London", "Paris", "Berlin", "Helsinki", "Tokyo", "New York", "Dubai", "Rome"}
	}
}

func (g *Game) refreshLeaderboard() {
	scores, stats, err := g.dataManager.GetLeaderboard()
	if err == nil {
		g.highScores = scores
		g.userStatsList = stats
	}
}

func (g *Game) refreshFlights() {
	for {
		flights, err := g.flightClient.FetchFlights(myLat, myLon, 1.0)
		if err != nil {
			log.Println("Error fetching flights:", err)
		} else {
			g.flights = flights
			// Update selected/target references if they still exist
			if g.selectedPlane != nil {
				found := false
				for _, f := range flights {
					if f.Icao24 == g.selectedPlane.Icao24 {
						g.selectedPlane = &f
						found = true
						break
					}
				}
				if !found {
					// Plane disappeared
				}
			}
			if g.targetPlane != nil {
				for _, f := range flights {
					if f.Icao24 == g.targetPlane.Icao24 {
						g.targetPlane = &f
						break
					}
				}
			}
		}
		time.Sleep(5 * time.Second) // Faster polling
	}
}

// getLogicalCursorPosition returns the game logic coordinates (Landscape)
// derived from physical screen coordinates (Portrait)
func (g *Game) getLogicalCursorPosition() (int, int) {
	var x, y int
	// Check touches first
	ids := ebiten.AppendTouchIDs(nil)
	if len(ids) > 0 {
		x, y = ebiten.TouchPosition(ids[0])
	} else {
		x, y = ebiten.CursorPosition()
	}

	// Remap Logic:
	// Physical X (0-720) becomes Game Y (0-720)
	// Physical Y (0-1280) becomes Game X (0-1280)

	// GameX = PhysicalY
	// GameY = ScreenWidth - PhysicalX (physicalWidth is 720)

	return y, physicalWidth - x
}

func (g *Game) Update() error {
	// handle quit request
	if g.shouldQuit {
		return ebiten.Termination
	}

	// Text Input for Login
	if g.state == StateLogin {
		if !g.showDeleteConfirm {
			g.inputText += string(ebiten.InputChars())
			if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
				if len(g.inputText) > 0 {
					g.inputText = g.inputText[:len(g.inputText)-1]
				}
			}
			if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
				if len(g.inputText) > 0 {
					g.login(g.inputText)
				}
			}
		}
	}

	// 1. Handle Pinch-to-Zoom (Two Fingers)
	touchIDs := ebiten.AppendTouchIDs(nil)
	if len(touchIDs) == 2 {
		// Get raw physical positions of both fingers
		x1, y1 := ebiten.TouchPosition(touchIDs[0])
		x2, y2 := ebiten.TouchPosition(touchIDs[1])

		// Calculate distance between fingers (Pythagorean theorem)
		// We use physical coordinates; rotation doesn't change distance.
		currentDist := math.Hypot(float64(x2-x1), float64(y2-y1))

		if g.lastPinchDist > 0 {
			// Sensitivity threshold (pixels) to prevent jitter
			threshold := 10.0
			diff := currentDist - g.lastPinchDist

			// If fingers moved enough to warrant a zoom change
			if math.Abs(diff) > threshold {
				if diff > 0 {
					g.camZoom++ // Spread fingers = Zoom In
				} else {
					g.camZoom-- // Pinch fingers = Zoom Out
				}

				// Clamp Zoom
				if g.camZoom < 4 {
					g.camZoom = 4
				}
				if g.camZoom > 18 {
					g.camZoom = 18
				}

				// Reset baseline to current to avoid rapid-fire zooming
				g.lastPinchDist = currentDist
			}
		} else {
			// First frame of the pinch, just establish baseline
			g.lastPinchDist = currentDist
		}
		// Disable dragging while pinching to prevent map jumping
		g.isDragging = false
		return nil
	} else {
		// Reset pinch distance if not exactly 2 fingers
		g.lastPinchDist = 0
	}

	// 2. Touch/Mouse Pan (One Finger / Mouse)
	// Only pan if we aren't zooming
	justPressed := inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) || (len(inpututil.JustPressedTouchIDs()) > 0 && len(touchIDs) == 1)

	if justPressed {
		g.isDragging = true
		g.dragStartX, g.dragStartY = g.getLogicalCursorPosition()
		g.startCamLat, g.startCamLon = g.camLat, g.camLon

		// Check click on planes/UI
		if !g.checkUIClick(g.dragStartX, g.dragStartY) {
			if g.state == StateMap || g.state == StateGamePlaying {
				g.checkPlaneClick(g.dragStartX, g.dragStartY)
			}
		} else {
			// UI clicked, cancel drag
			g.isDragging = false
		}
	}

	// Check if Held
	isHeld := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft) || len(touchIDs) == 1

	if g.isDragging {
		if isHeld {
			currX, currY := g.getLogicalCursorPosition()
			dx := currX - g.dragStartX
			dy := currY - g.dragStartY

			// Only pan in Map/Game mode
			if g.state == StateMap || g.state == StateGamePlaying {
				// Convert pixels to lat/lon delta
				scale := 360.0 / math.Pow(2, float64(g.camZoom)) / 256.0
				g.camLon = g.startCamLon - float64(dx)*scale
				latScale := scale * math.Cos(g.camLat*math.Pi/180.0)
				g.camLat = g.startCamLat + float64(dy)*latScale
			}
		} else {
			g.isDragging = false
		}
	}

	// 3. Mouse Wheel Zoom (Keep this for desktop testing)
	_, wheelDy := ebiten.Wheel()
	if wheelDy != 0 {
		g.camZoom += int(wheelDy)
		if g.camZoom < 4 {
			g.camZoom = 4
		}
		if g.camZoom > 18 {
			g.camZoom = 18
		}
	}

	// Game Logic Transitions
	if g.state == StateGamePlaying && g.showResult {
		if time.Since(g.resultStartTime) > 2*time.Second {
			g.nextRound()
		}
	}

	return nil
}

func (g *Game) login(name string) {
	if u, ok := g.usersMap[name]; ok {
		g.currentUser = u
	} else {
		g.currentUser = UserStats{Name: name}
	}
	g.state = StateMap
}

func (g *Game) checkUIClick(x, y int) bool {
	// Iterate buttons in reverse (topmost first)
	for i := len(g.buttons) - 1; i >= 0; i-- {
		b := g.buttons[i]
		if x >= b.X && x <= b.X+b.W && y >= b.Y && y <= b.Y+b.H {
			if b.Action != nil {
				b.Action()
			}
			return true
		}
	}
	// Also catch clicks on sidebars to prevent map panning through them
	if g.selectedPlane != nil && x > logicalWidth-300 {
		return true
	}
	if g.state == StateGamePlaying && x < 300 {
		return true
	}
	return false
}

// selectPlane handles selection logic including firing the scraper
func (g *Game) selectPlane(f *Flight) {
	g.selectedPlane = f
	g.resolvedDetails = nil
	g.resolving = true

	// Trigger scrape
	go func(callsign string) {
		details, err := g.scraper.FetchFlightDetails(callsign)
		if err != nil {
			log.Printf("Failed to resolve %s: %v", callsign, err)
			g.resolving = false
			return
		}
		// Store scraped airports for future use
		if details != nil {
			// Save async
			go func() {
				g.dataManager.SaveAirport(details.RealDestination)
				g.dataManager.SaveAirport(details.Origin)
			}()
		}

		// Only update if selection hasn't changed
		if g.selectedPlane != nil && g.selectedPlane.Callsign == callsign {
			g.resolvedDetails = details
			g.resolving = false
		}
	}(f.Callsign)
}

func (g *Game) checkPlaneClick(x, y int) {
	// Find closest plane
	minDist := 40.0 // Click radius
	var found *Flight

	centerX, centerY := LatLonToPixels(g.camLat, g.camLon, g.camZoom)
	screenCX, screenCY := float64(logicalWidth)/2, float64(logicalHeight)/2
	minWX := centerX - screenCX
	minWY := centerY - screenCY

	for i := range g.flights {
		f := &g.flights[i]
		fX, fY := LatLonToPixels(f.Lat, f.Lon, g.camZoom)
		sX := fX - minWX
		sY := fY - minWY

		dist := math.Sqrt(math.Pow(sX-float64(x), 2) + math.Pow(sY-float64(y), 2))
		if dist < minDist {
			minDist = dist
			found = f
		}
	}

	if found != nil {
		g.selectPlane(found)

		// Auto-center if game is not active
		if g.state == StateMap {
			g.camLat = found.Lat
			g.camLon = found.Lon
		}
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Draw logic to offscreen buffer (Landscape)
	g.offscreen.Fill(color.RGBA{15, 23, 42, 255})

	if g.state == StateLogin {
		g.drawLogin(g.offscreen)
	} else if g.state == StateLeaderboard {
		g.drawLeaderboard(g.offscreen)
	} else {
		g.drawMap(g.offscreen)
		g.drawHomeMarker(g.offscreen)
		g.drawPlanes(g.offscreen)
		g.drawUI(g.offscreen)
	}

	// Render offscreen to physical screen with rotation
	op := &ebiten.DrawImageOptions{}

	// 1. Move image to center so we rotate around the center
	op.GeoM.Translate(-float64(logicalWidth)/2, -float64(logicalHeight)/2)

	// 2. Rotate 90 degrees (Pi/2 radians)
	op.GeoM.Rotate(math.Pi / 2)

	// 3. Move back to center of the destination screen
	op.GeoM.Translate(float64(physicalWidth)/2, float64(physicalHeight)/2)

	screen.DrawImage(g.offscreen, op)
}

func (g *Game) drawLogin(screen *ebiten.Image) {
	g.buttons = []Button{}

	title := "VANTAA FLIGHTRADAR24"
	titleW := len(title) * 7 * 2 // approx scale 2
	// Use scale for title
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(2, 2)
	op.GeoM.Translate(float64(logicalWidth-titleW)/2, 80)
	text.Draw(screen, title, basicfont.Face7x13, (logicalWidth/2)-(len(title)*7), 100, hexToColor(colAccent))

	if g.showDeleteConfirm {
		// Confirmation Dialog
		ebitenutil.DrawRect(screen, float64(logicalWidth/2-150), 200, 300, 150, hexToColor(colGlass))
		text.Draw(screen, fmt.Sprintf("Delete user '%s'?", g.userToDelete), basicfont.Face7x13, logicalWidth/2-130, 240, color.White)
		text.Draw(screen, "This cannot be undone.", basicfont.Face7x13, logicalWidth/2-130, 260, hexToColor(colDanger))

		g.addButton(logicalWidth/2-110, 290, 100, 30, "CANCEL", func() {
			g.showDeleteConfirm = false
			g.userToDelete = ""
		}, hexToColor(colGlassLight))

		g.addButton(logicalWidth/2+10, 290, 100, 30, "DELETE", func() {
			g.dataManager.DeleteUser(g.userToDelete)
			g.refreshUsers()
			g.showDeleteConfirm = false
			g.userToDelete = ""
		}, hexToColor(colDanger))

	} else {
		text.Draw(screen, "Select User or Type Name:", basicfont.Face7x13, logicalWidth/2-100, 160, color.White)

		// Input Box
		ebitenutil.DrawRect(screen, float64(logicalWidth/2-100), 180, 200, 30, color.White)
		text.Draw(screen, g.inputText, basicfont.Face7x13, logicalWidth/2-95, 200, color.Black)

		if len(g.inputText) > 0 {
			g.addButton(logicalWidth/2+110, 180, 60, 30, "GO", func() { g.login(g.inputText) }, hexToColor(colSuccess))
		}

		// User List
		y := 240
		keys := make([]string, 0, len(g.usersMap))
		for k := range g.usersMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		for _, name := range keys {
			u := g.usersMap[name]
			label := fmt.Sprintf("%s (Best: %d)", u.Name, u.BestScore)
			// Capture loop var
			n := name

			// User button
			g.addButton(logicalWidth/2-100, y, 200, 30, label, func() { g.login(n) }, hexToColor(colGlassLight))

			// Delete button
			g.addButton(logicalWidth/2+110, y, 30, 30, "X", func() {
				g.userToDelete = n
				g.showDeleteConfirm = true
			}, hexToColor(colDanger))

			y += 40
		}
	}

	// Add a bottom-left EXIT button on the login screen
	g.addButton(20, logicalHeight-50, 100, 30, "QUIT", func() {
		g.shouldQuit = true
	}, hexToColor(colDanger))

	// Draw buttons
	for _, b := range g.buttons {
		ebitenutil.DrawRect(screen, float64(b.X), float64(b.Y), float64(b.W), float64(b.H), b.Color)
		tW := len(b.Text) * 7
		text.Draw(screen, b.Text, basicfont.Face7x13, b.X+(b.W-tW)/2, b.Y+b.H/2+4, b.TextColor)
	}
}

func (g *Game) drawLeaderboard(screen *ebiten.Image) {
	g.buttons = []Button{}

	text.Draw(screen, "LEADERBOARD", basicfont.Face7x13, 20, 30, hexToColor(colAccent))

	// High Scores Column
	text.Draw(screen, "TOP SCORES", basicfont.Face7x13, 50, 70, color.White)
	y := 100
	for i, s := range g.highScores {
		line := fmt.Sprintf("%d. %s - %d", i+1, s.Name, s.Score)
		text.Draw(screen, line, basicfont.Face7x13, 50, y, color.White)
		y += 25
	}

	// User Stats Column
	text.Draw(screen, "PLAYER STATS", basicfont.Face7x13, 400, 70, color.White)
	y = 100
	for i, u := range g.userStatsList {
		if i >= 10 {
			break
		}
		line := fmt.Sprintf("%s: Best %d | Played %d | Perf %d%%", u.Name, u.BestScore, u.GamesPlayed, u.PerformancePercent)
		text.Draw(screen, line, basicfont.Face7x13, 400, y, color.White)
		y += 25
	}

	g.addButton(20, logicalHeight-50, 100, 30, "BACK", func() { g.state = StateMap }, hexToColor(colDanger))

	// Draw buttons
	for _, b := range g.buttons {
		ebitenutil.DrawRect(screen, float64(b.X), float64(b.Y), float64(b.W), float64(b.H), b.Color)
		tW := len(b.Text) * 7
		text.Draw(screen, b.Text, basicfont.Face7x13, b.X+(b.W-tW)/2, b.Y+b.H/2+4, b.TextColor)
	}
}

func (g *Game) drawMap(screen *ebiten.Image) {
	centerX, centerY := LatLonToPixels(g.camLat, g.camLon, g.camZoom)
	screenCX, screenCY := float64(logicalWidth)/2, float64(logicalHeight)/2
	minWX := centerX - screenCX
	minWY := centerY - screenCY

	minTileX := int(math.Floor(minWX / tileSize))
	maxTileX := int(math.Floor((centerX + screenCX) / tileSize))
	minTileY := int(math.Floor(minWY / tileSize))
	maxTileY := int(math.Floor((centerY + screenCY) / tileSize))

	maxIndex := int(math.Pow(2, float64(g.camZoom))) - 1

	for x := minTileX; x <= maxTileX; x++ {
		for y := minTileY; y <= maxTileY; y++ {
			tileX := x
			for tileX < 0 {
				tileX += maxIndex + 1
			}
			for tileX > maxIndex {
				tileX -= maxIndex + 1
			}

			if y < 0 || y > maxIndex {
				continue
			}

			img := g.tileLoader.GetTile(g.camZoom, tileX, y)
			if img != nil {
				screenX := float64(x*tileSize) - minWX
				screenY := float64(y*tileSize) - minWY
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Translate(screenX, screenY)
				screen.DrawImage(img, op)
			}
		}
	}
}

func (g *Game) drawHomeMarker(screen *ebiten.Image) {
	centerX, centerY := LatLonToPixels(g.camLat, g.camLon, g.camZoom)
	screenCX, screenCY := float64(logicalWidth)/2, float64(logicalHeight)/2
	minWX := centerX - screenCX
	minWY := centerY - screenCY

	hX, hY := LatLonToPixels(myLat, myLon, g.camZoom)
	sX := hX - minWX
	sY := hY - minWY

	if sX >= 0 && sX <= float64(logicalWidth) && sY >= 0 && sY <= float64(logicalHeight) {
		// Draw a dot (6x6 square)
		size := 6.0
		ebitenutil.DrawRect(screen, sX-size/2, sY-size/2, size, size, hexToColor(colAccent))
	}
}

func (g *Game) drawPlanes(screen *ebiten.Image) {
	centerX, centerY := LatLonToPixels(g.camLat, g.camLon, g.camZoom)
	screenCX, screenCY := float64(logicalWidth)/2, float64(logicalHeight)/2
	minWX := centerX - screenCX
	minWY := centerY - screenCY

	for _, f := range g.flights {
		fX, fY := LatLonToPixels(f.Lat, f.Lon, g.camZoom)
		sX := fX - minWX
		sY := fY - minWY

		if sX < -50 || sX > float64(logicalWidth)+50 || sY < -50 || sY > float64(logicalHeight)+50 {
			continue
		}

		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(-16, -16)
		op.GeoM.Rotate(f.Heading * math.Pi / 180.0)
		op.GeoM.Translate(sX, sY)

		// Highlight target
		if g.state == StateGamePlaying && g.targetPlane != nil && f.Icao24 == g.targetPlane.Icao24 {
			op.ColorScale.Scale(1, 0.8, 0.2, 1) // Orange tint
		}

		screen.DrawImage(g.planeImg, op)

		// Label
		text.Draw(screen, f.Callsign, basicfont.Face7x13, int(sX)+20, int(sY), color.White)
	}
}

func (g *Game) drawUI(screen *ebiten.Image) {
	g.buttons = []Button{} // Reset buttons from previous frame

	// Top Bar: User info
	if g.state == StateMap {
		text.Draw(screen, fmt.Sprintf("User: %s (Best: %d)", g.currentUser.Name, g.currentUser.BestScore), basicfont.Face7x13, 10, 20, hexToColor(colAccent))
		g.addButton(logicalWidth-110, 10, 100, 30, "LEADERBOARD", func() {
			g.refreshLeaderboard()
			g.state = StateLeaderboard
		}, hexToColor(colGlass))
		g.addButton(logicalWidth-220, 10, 100, 30, "LOGOUT", func() { g.state = StateLogin; g.inputText = "" }, hexToColor(colDanger))
	}

	// Sidebar (Right) - Plane Info
	if g.selectedPlane != nil {
		g.drawPanel(screen, logicalWidth-300, 90, 280, 350, "FLIGHT INFO")

		// Content
		p := g.selectedPlane
		y := 140
		text.Draw(screen, p.Callsign, basicfont.Face7x13, logicalWidth-280, y, hexToColor(colAccent))
		y += 30
		text.Draw(screen, fmt.Sprintf("Alt: %d ft", p.AltitudeFt), basicfont.Face7x13, logicalWidth-280, y, color.White)
		y += 20
		text.Draw(screen, fmt.Sprintf("Spd: %d kts", p.VelocityKts), basicfont.Face7x13, logicalWidth-280, y, color.White)
		y += 20
		text.Draw(screen, fmt.Sprintf("Lat/Lon: %.2f, %.2f", p.Lat, p.Lon), basicfont.Face7x13, logicalWidth-280, y, color.White)

		y += 30
		// Extended Details
		if g.resolving {
			text.Draw(screen, "Fetching details...", basicfont.Face7x13, logicalWidth-280, y, hexToColor(colTextMuted))
		} else if g.resolvedDetails != nil {
			text.Draw(screen, "Model: "+truncate(g.resolvedDetails.Model, 35), basicfont.Face7x13, logicalWidth-280, y, color.White)

			// Masking logic: If we are playing and this is the target, hide the answer
			showOrigin := g.resolvedDetails.Origin
			showDest := g.resolvedDetails.RealDestination

			if g.state == StateGamePlaying && g.targetPlane != nil && g.selectedPlane.Icao24 == g.targetPlane.Icao24 {
				// Hide answer based on question type
				// If correct option matches one of these, hide it
				if g.correctOption == g.resolvedDetails.Origin {
					showOrigin = "???"
				}
				if g.correctOption == g.resolvedDetails.RealDestination {
					showDest = "???"
				}
			}

			y += 20
			text.Draw(screen, "Origin: "+truncate(showOrigin, 25), basicfont.Face7x13, logicalWidth-280, y, color.White)
			y += 20
			text.Draw(screen, "Dest: "+truncate(showDest, 25), basicfont.Face7x13, logicalWidth-280, y, color.White)
		} else {
			text.Draw(screen, "Details unavailable", basicfont.Face7x13, logicalWidth-280, y, hexToColor(colTextMuted))
		}

		// Close Button
		g.addButton(logicalWidth-50, 95, 30, 30, "X", func() { g.selectedPlane = nil }, color.RGBA{255, 255, 255, 50}, color.Black)
	}

	// Game Panel (Left)
	if g.state == StateRoundSetup {
		g.drawPanel(screen, 20, 90, 280, 150, fmt.Sprintf("ROUND %d/5", g.round))
		text.Draw(screen, "Tracking target...", basicfont.Face7x13, 40, 140, color.White)
		text.Draw(screen, "Please wait", basicfont.Face7x13, 40, 160, hexToColor(colTextMuted))
	} else if g.state == StateGamePlaying && g.targetPlane != nil {
		g.drawPanel(screen, 20, 90, 280, 340, fmt.Sprintf("ROUND %d/5", g.round))

		text.Draw(screen, g.questionText, basicfont.Face7x13, 40, 140, color.White)

		// Options
		y := 170
		for _, opt := range g.options {
			col := hexToColor(0xffffff20) // Default transparent white

			// Feedback colors
			if g.showResult {
				if opt == g.correctOption {
					col = hexToColor(colSuccess)
				} else if !g.resultCorrect && opt == g.wrongGuess {
					col = hexToColor(colDanger) // Highlight wrong guess red
				}
			}

			// Capture variable for closure
			btnOpt := opt
			g.addButton(40, y, 240, 40, opt, func() { g.guess(btnOpt) }, col, color.Black)
			y += 50
		}

		// Score
		text.Draw(screen, fmt.Sprintf("Score: %d", g.score), basicfont.Face7x13, 40, y+20, hexToColor(colAccent))

		y += 40 // Add margin after the score

		// Quit Button
		g.addButton(20, 400, 100, 30, "QUIT", func() { g.endGame() }, hexToColor(colDanger))
	}

	// Bottom Controls
	if g.state == StateMap {
		g.addButton(logicalWidth/2-60, logicalHeight-60, 120, 40, "PLAY GAME", func() { g.startGame() }, hexToColor(colAccent))
		g.addButton(20, logicalHeight-60, 80, 40, "CENTER", func() {
			g.camLat = myLat
			g.camLon = myLon
		}, hexToColor(colGlass))
	} else if g.state == StateGameOver {
		g.drawPanel(screen, logicalWidth/2-150, logicalHeight/2-100, 300, 200, "GAME OVER")
		text.Draw(screen, fmt.Sprintf("Final Score: %d", g.score), basicfont.Face7x13, logicalWidth/2-50, logicalHeight/2, color.White)
		g.addButton(logicalWidth/2-60, logicalHeight/2+40, 120, 40, "CLOSE", func() { g.endGame() }, hexToColor(colAccent))
	}

	// Register Buttons in UI pass
	for _, b := range g.buttons {
		ebitenutil.DrawRect(screen, float64(b.X), float64(b.Y), float64(b.W), float64(b.H), b.Color)
		// Center text roughly
		tW := len(b.Text) * 7
		text.Draw(screen, b.Text, basicfont.Face7x13, b.X+(b.W-tW)/2, b.Y+b.H/2+4, b.TextColor)
	}

	ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS: %0.2f", ebiten.ActualFPS()))
}

func (g *Game) drawPanel(screen *ebiten.Image, x, y, w, h int, title string) {
	// Background
	ebitenutil.DrawRect(screen, float64(x), float64(y), float64(w), float64(h), hexToColor(colGlass))
	// Title
	text.Draw(screen, title, basicfont.Face7x13, x+20, y+30, hexToColor(colAccent))
}

func (g *Game) addButton(x, y, w, h int, label string, action func(), col color.Color, txtCol ...color.Color) {
	textColor := color.Color(color.White)
	if len(txtCol) > 0 {
		textColor = txtCol[0]
	}
	g.buttons = append(g.buttons, Button{X: x, Y: y, W: w, H: h, Text: label, Action: action, Color: col, TextColor: textColor})
}

func (g *Game) startGame() {
	if len(g.flights) == 0 {
		return
	}
	g.score = 0
	g.round = 0
	g.nextRound()
}

func (g *Game) endGame() {
	// Save stats only if round > 0 and user played
	if g.round > 0 {
		u, err := g.dataManager.SaveUser(g.currentUser.Name, g.score)
		if err == nil {
			g.currentUser = u      // update local ref
			g.usersMap[u.Name] = u // update list ref
		} else {
			log.Println("Error saving user:", err)
		}

		_, err = g.dataManager.AddScore(ScoreEntry{
			Name:  g.currentUser.Name,
			Score: g.score,
			Date:  time.Now().Format("2006-01-02"),
		})
		if err != nil {
			log.Println("Error saving score:", err)
		}
	}

	g.state = StateMap
	g.selectedPlane = nil
}

func (g *Game) nextRound() {
	g.round++
	if g.round > 5 {
		g.state = StateGameOver
		return
	}

	g.pickNewTarget()
}

func (g *Game) pickNewTarget() {
	g.state = StateRoundSetup
	g.showResult = false
	g.wrongGuess = ""

	if len(g.flights) == 0 {
		// No flights, wait and retry?
		// For simplicity, let's just reset state or wait.
		// Since this is async, we can just re-schedule.
		// But g.flights is updated by another goroutine.
		// Let's just retry in 1 sec.
		time.AfterFunc(1*time.Second, g.pickNewTarget)
		return
	}

	idx := rand.Intn(len(g.flights))
	g.targetPlane = &g.flights[idx]

	g.camLat = g.targetPlane.Lat
	g.camLon = g.targetPlane.Lon

	g.selectedPlane = g.targetPlane
	g.resolvedDetails = nil
	g.resolving = true

	go func() {
		details, err := g.scraper.FetchFlightDetails(g.targetPlane.Callsign)

		if err == nil && details != nil {
			g.setupRoundWithData(details)
		} else {
			log.Println("Scrape failed, trying new target:", err)
			g.pickNewTarget()
		}
	}()
}

func (g *Game) setupRoundWithData(details *ResolvedDetails) {
	g.resolvedDetails = details
	g.resolving = false

	// Validate Data - must not be Unknown or empty
	if details.RealDestination == "" || details.RealDestination == "Unknown" ||
		details.Origin == "" || details.Origin == "Unknown" {
		log.Println("Invalid data (Unknown), trying new target")
		g.pickNewTarget()
		return
	}

	g.dataManager.SaveAirport(details.RealDestination)
	g.dataManager.SaveAirport(details.Origin)

	origin := details.Origin
	dest := details.RealDestination

	isInbound := strings.Contains(dest, "Helsinki") || strings.Contains(dest, "Vantaa")

	if isInbound {
		g.questionText = fmt.Sprintf("Where is %s from?", g.targetPlane.Callsign)
		g.correctOption = origin
	} else {
		g.questionText = fmt.Sprintf("Where is %s going?", g.targetPlane.Callsign)
		g.correctOption = dest
	}

	g.generateOptions()
	g.roundStartTime = time.Now()
	g.state = StateGamePlaying
}

func (g *Game) setupRoundFallback() {
	// Deprecated fallback - we prefer to retry scraping another plane.
	// If we really want fallback, we can uncomment.
	// For now, just retry new target.
	g.pickNewTarget()
}

func (g *Game) generateOptions() {
	g.refreshAirports()

	distractors := make([]string, len(g.airports))
	copy(distractors, g.airports)

	rand.Shuffle(len(distractors), func(i, j int) {
		distractors[i], distractors[j] = distractors[j], distractors[i]
	})

	opts := []string{g.correctOption}
	for _, c := range distractors {
		if len(opts) >= 4 {
			break
		}
		if c != g.correctOption && c != "Unknown" {
			opts = append(opts, c)
		}
	}

	// Fill if needed
	if len(opts) < 4 {
		fallbacks := []string{"London", "Paris", "Berlin", "Helsinki", "Tokyo", "New York"}
		for _, c := range fallbacks {
			if len(opts) >= 4 {
				break
			}
			exists := false
			for _, o := range opts {
				if o == c {
					exists = true
					break
				}
			}
			if !exists {
				opts = append(opts, c)
			}
		}
	}

	rand.Shuffle(len(opts), func(i, j int) {
		opts[i], opts[j] = opts[j], opts[i]
	})
	g.options = opts
}

func (g *Game) guess(city string) {
	if g.showResult {
		return
	}

	g.resultCorrect = (city == g.correctOption)
	if g.resultCorrect {
		// Time bonus
		elapsed := time.Since(g.roundStartTime).Seconds()
		bonus := int(math.Max(0, (20.0-elapsed)/20.0*100.0))
		g.score += 100 + bonus
	} else {
		g.wrongGuess = city
	}
	g.showResult = true
	g.resultStartTime = time.Now()
}

func hexToColor(hex uint32) color.Color {
	return color.RGBA{
		R: uint8(hex >> 24),
		G: uint8(hex >> 16),
		B: uint8(hex >> 8),
		A: uint8(hex),
	}
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return physicalWidth, physicalHeight
}

func main() {
	if l := os.Getenv("MY_LAT"); l != "" {
		if v, err := strconv.ParseFloat(l, 64); err == nil {
			myLat = v
		}
	}
	if l := os.Getenv("MY_LON"); l != "" {
		if v, err := strconv.ParseFloat(l, 64); err == nil {
			myLon = v
		}
	}

	// Initialize flight client with auth and caching
	client := NewFlightClient()

	// Start the Game
	game := NewGame(client)
	ebiten.SetWindowSize(physicalWidth, physicalHeight)
	ebiten.SetWindowTitle("Flight Monitor (Rotated)")

	ebiten.SetTPS(24)
	ebiten.SetFullscreen(true)

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal(err)
	}
}

func createPlaneImage() *ebiten.Image {
	img := ebiten.NewImage(32, 32)
	whiteSubImage := ebiten.NewImage(1, 1)
	whiteSubImage.Fill(color.White)
	clr := color.RGBA{56, 189, 248, 255}
	r, g, b, a := clr.RGBA()
	cr, cg, cb, ca := float32(r)/65535, float32(g)/65535, float32(b)/65535, float32(a)/65535
	vertices := []ebiten.Vertex{
		{DstX: 16, DstY: 2, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
		{DstX: 28, DstY: 28, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
		{DstX: 16, DstY: 22, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
		{DstX: 4, DstY: 28, ColorR: cr, ColorG: cg, ColorB: cb, ColorA: ca},
	}
	indices := []uint16{0, 1, 2, 0, 2, 3}
	img.DrawTriangles(vertices, indices, whiteSubImage, nil)
	return img
}
