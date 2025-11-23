package main

import (
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	rl "github.com/gen2brain/raylib-go/raylib"
)

const (
	// Physical screen dimensions (Portrait)
	// We'll treat the screen as natively landscape for simplicity if rotated via config,
	// or we can stick to the 800x600 user mentioned.
	// Let's assume the user wants the 854x480 logical size we settled on.
	screenWidth  = 1280
	screenHeight = 720

	defaultZoom = 11

	// UI Colors
	colBgDark     = 0x0f172aff // #0f172a
	colAccent     = 0x38bdf8ff // #38bdf8
	colGlass      = 0x0f172af2 // 95% opacity
	colGlassLight = 0x334155ff // #334155
	colText       = 0xf1f5f9ff // #f1f5f9
	colTextMuted  = 0x94a3b8ff // #94a3b8
	colSuccess    = 0x4ade80ff // #4ade80
	colDanger     = 0xf87171ff // #f87171
)

func getRlColor(hex uint32) rl.Color {
	return rl.Color{
		R: uint8(hex >> 24),
		G: uint8(hex >> 16),
		B: uint8(hex >> 8),
		A: uint8(hex),
	}
}

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
	StateRoundSetup
	StateGameOver
	StateLeaderboard
)

type Button struct {
	X, Y, W, H int
	Text       string
	Action     func()
	Color      rl.Color
	TextColor  rl.Color
}

type Game struct {
	flightClient *FlightClient
	tileLoader   *TileLoader
	dataManager  *DataManager
	scraper      *Scraper
	flights      []Flight
	state        State
	shouldQuit   bool

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
	isKeyboardOpen    bool
	keyboardLayout    []string

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
	planeTex rl.Texture2D

	// Selected Plane
	selectedPlane   *Flight
	resolvedDetails *ResolvedDetails
	resolving       bool

	// Game Logic
	score           int
	targetPlane     *Flight
	round           int
	roundStartTime  time.Time
	questionText    string
	options         []string
	correctOption   string
	wrongGuess      string
	showResult      bool
	resultCorrect   bool
	resultStartTime time.Time

	// UI Elements
	buttons []Button
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
		state:        StateLogin,
		keyboardLayout: []string{
			"QWERTYUIOP",
			"ASDFGHJKL",
			"ZXCVBNM-",
		},
	}

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
			// Update selected/target references
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
		time.Sleep(5 * time.Second)
	}
}

// createPlaneTexture generates a simple plane sprite
func createPlaneTexture() rl.Texture2D {
	img := rl.GenImageColor(32, 32, rl.Blank)

	// Draw a detailed plane shape similar to the Ebiten version
	// Center is 16,16

	// Main Body (Fuselage)
	rl.ImageDrawRectangle(img, 14, 2, 4, 26, rl.White)

	// Wings (Triangle shape)
	// Left Wing
	rl.ImageDrawTriangle(img,
		rl.Vector2{X: 14, Y: 12},
		rl.Vector2{X: 2, Y: 20},
		rl.Vector2{X: 14, Y: 20},
		rl.White)
	// Right Wing
	rl.ImageDrawTriangle(img,
		rl.Vector2{X: 18, Y: 12},
		rl.Vector2{X: 18, Y: 20},
		rl.Vector2{X: 30, Y: 20},
		rl.White)

	// Tail
	rl.ImageDrawRectangle(img, 10, 24, 12, 3, rl.White)

	tex := rl.LoadTextureFromImage(img)
	rl.UnloadImage(img)
	return tex
}

func (g *Game) Init() {
	g.planeTex = createPlaneTexture()
	// Set texture filter to Point for crisp text if using default font at integer scales
	// rl.SetTextureFilter(rl.GetFontDefault().Texture, rl.TextureFilterPoint)
}

func (g *Game) Unload() {
	rl.UnloadTexture(g.planeTex)
	g.tileLoader.Unload()
}

func (g *Game) Update() {
	// 1. Text Input
	if g.state == StateLogin && !g.showDeleteConfirm {
		key := rl.GetCharPressed()
		for key > 0 {
			g.inputText += string(key)
			key = rl.GetCharPressed()
		}
		if rl.IsKeyPressed(rl.KeyBackspace) {
			if len(g.inputText) > 0 {
				g.inputText = g.inputText[:len(g.inputText)-1]
			}
		}
		if rl.IsKeyPressed(rl.KeyEnter) {
			if len(g.inputText) > 0 {
				g.login(g.inputText)
			}
		}
	}

	// 2. Pinch Zoom
	// Raylib Touch
	touchCount := rl.GetTouchPointCount()
	if !g.isKeyboardOpen && touchCount == 2 {
		t1 := rl.GetTouchPosition(0)
		t2 := rl.GetTouchPosition(1)

		dist := math.Sqrt(math.Pow(float64(t2.X-t1.X), 2) + math.Pow(float64(t2.Y-t1.Y), 2))

		if g.lastPinchDist > 0 {
			threshold := 10.0
			diff := dist - g.lastPinchDist
			if math.Abs(diff) > threshold {
				if diff > 0 {
					g.camZoom++
				} else {
					g.camZoom--
				}
				if g.camZoom < 4 {
					g.camZoom = 4
				}
				if g.camZoom > 18 {
					g.camZoom = 18
				}
				g.lastPinchDist = dist
			}
		} else {
			g.lastPinchDist = dist
		}
		g.isDragging = false
		return // Skip other input
	} else {
		g.lastPinchDist = 0
	}

	// 3. Pan / Click
	// Mouse or Single Touch
	isClick := rl.IsMouseButtonPressed(rl.MouseLeftButton)
	isDown := rl.IsMouseButtonDown(rl.MouseLeftButton)

	// Get position (Mouse or Touch 0)
	// Raylib handles this unification mostly, but let's be explicit
	mPos := rl.GetMousePosition()
	mx, my := int(mPos.X), int(mPos.Y)

	if isClick {
		g.isDragging = true
		g.dragStartX = mx
		g.dragStartY = my
		g.startCamLat = g.camLat
		g.startCamLon = g.camLon

		if g.checkUIClick(mx, my) {
			g.isDragging = false
		} else {
			if g.isKeyboardOpen {
				g.isDragging = false
			} else if g.state == StateMap || g.state == StateGamePlaying {
				g.checkPlaneClick(mx, my)
			}
		}
	}

	if g.isDragging {
		if isDown {
			dx := mx - g.dragStartX
			dy := my - g.dragStartY

			if g.state == StateMap || g.state == StateGamePlaying {
				// Pan Logic
				scale := 360.0 / math.Pow(2, float64(g.camZoom)) / 256.0
				g.camLon = g.startCamLon - float64(dx)*scale
				latScale := scale * math.Cos(g.camLat*math.Pi/180.0)
				g.camLat = g.startCamLat + float64(dy)*latScale
			}
		} else {
			g.isDragging = false
		}
	}

	// Mouse Wheel
	wheel := rl.GetMouseWheelMove()
	if wheel != 0 {
		g.camZoom += int(wheel)
		if g.camZoom < 4 {
			g.camZoom = 4
		}
		if g.camZoom > 18 {
			g.camZoom = 18
		}
	}

	// Fullscreen Toggle
	if rl.IsKeyPressed(rl.KeyF) {
		rl.ToggleFullscreen()
	}

	// Game State Transitions
	if g.state == StateGamePlaying && g.showResult {
		if time.Since(g.resultStartTime) > 2*time.Second {
			g.nextRound()
		}
	}

	// Update Tile Loader
	g.tileLoader.Update()
}

func (g *Game) login(name string) {
	g.isKeyboardOpen = false
	if u, ok := g.usersMap[name]; ok {
		g.currentUser = u
	} else {
		g.currentUser = UserStats{Name: name}
	}
	g.state = StateMap
}

func (g *Game) checkUIClick(x, y int) bool {
	// Reverse iterate
	for i := len(g.buttons) - 1; i >= 0; i-- {
		b := g.buttons[i]
		if x >= b.X && x <= b.X+b.W && y >= b.Y && y <= b.Y+b.H {
			if b.Action != nil {
				b.Action()
			}
			return true
		}
	}

	if g.selectedPlane != nil && x > screenWidth-300 {
		return true
	}
	if g.state == StateGamePlaying && x < 300 {
		return true
	}

	return false
}

func (g *Game) checkPlaneClick(x, y int) {
	minDist := 40.0
	var found *Flight

	centerX, centerY := LatLonToPixels(g.camLat, g.camLon, g.camZoom)
	screenCX, screenCY := float64(screenWidth)/2, float64(screenHeight)/2
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
		if g.state == StateMap {
			g.camLat = found.Lat
			g.camLon = found.Lon
		}
	}
}

func (g *Game) selectPlane(f *Flight) {
	g.selectedPlane = f
	g.resolvedDetails = nil
	g.resolving = true

	go func(callsign string) {
		details, err := g.scraper.FetchFlightDetails(callsign)
		if err != nil {
			log.Printf("Failed to resolve %s: %v", callsign, err)
			g.resolving = false
			return
		}
		if details != nil {
			go func() {
				g.dataManager.SaveAirport(details.RealDestination)
				g.dataManager.SaveAirport(details.Origin)
			}()
		}
		if g.selectedPlane != nil && g.selectedPlane.Callsign == callsign {
			g.resolvedDetails = details
			g.resolving = false
		}
	}(f.Callsign)
}

func (g *Game) Draw() {
	rl.BeginDrawing()
	rl.ClearBackground(getRlColor(colBgDark))

	if g.state == StateLogin {
		g.drawLogin()
	} else if g.state == StateLeaderboard {
		g.drawLeaderboard()
	} else {
		g.drawMap()
		g.drawHomeMarker()
		g.drawPlanes()
		g.drawUI()
	}

	// Debug
	rl.DrawFPS(10, screenHeight-20)

	rl.EndDrawing()
}

func (g *Game) drawMap() {
	centerX, centerY := LatLonToPixels(g.camLat, g.camLon, g.camZoom)
	screenCX, screenCY := float64(screenWidth)/2, float64(screenHeight)/2
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

			tex := g.tileLoader.GetTile(g.camZoom, tileX, y)
			// Check if valid texture (id > 0)
			if tex.ID > 0 {
				screenX := float64(x*tileSize) - minWX
				screenY := float64(y*tileSize) - minWY

				rl.DrawTexture(tex, int32(screenX), int32(screenY), rl.White)
			}
		}
	}
}

func (g *Game) drawHomeMarker() {
	centerX, centerY := LatLonToPixels(g.camLat, g.camLon, g.camZoom)
	screenCX, screenCY := float64(screenWidth)/2, float64(screenHeight)/2
	minWX := centerX - screenCX
	minWY := centerY - screenCY

	hX, hY := LatLonToPixels(myLat, myLon, g.camZoom)
	sX := hX - minWX
	sY := hY - minWY

	if sX >= 0 && sX <= float64(screenWidth) && sY >= 0 && sY <= float64(screenHeight) {
		rl.DrawRectangle(int32(sX)-3, int32(sY)-3, 6, 6, getRlColor(colAccent))
	}
}

func (g *Game) drawPlanes() {
	centerX, centerY := LatLonToPixels(g.camLat, g.camLon, g.camZoom)
	screenCX, screenCY := float64(screenWidth)/2, float64(screenHeight)/2
	minWX := centerX - screenCX
	minWY := centerY - screenCY

	for _, f := range g.flights {
		fX, fY := LatLonToPixels(f.Lat, f.Lon, g.camZoom)
		sX := fX - minWX
		sY := fY - minWY

		if sX < -50 || sX > float64(screenWidth)+50 || sY < -50 || sY > float64(screenHeight)+50 {
			continue
		}

		// Rotation
		// Raylib rotation is in degrees.
		destRect := rl.Rectangle{X: float32(sX), Y: float32(sY), Width: 32, Height: 32}
		origin := rl.Vector2{X: 16, Y: 16} // Center of rotation

		tint := rl.White
		// g.state == StateGamePlaying && -- add this to set only for game mode
		if g.targetPlane != nil && f.Icao24 == g.targetPlane.Icao24 {
			tint = rl.Orange // Highlight
		}

		rl.DrawTexturePro(g.planeTex,
			rl.Rectangle{X: 0, Y: 0, Width: 32, Height: 32}, // Source
			destRect,
			origin,
			float32(f.Heading),
			tint)

		rl.DrawText(f.Callsign, int32(sX)+20, int32(sY), 10, rl.White)
	}
}

func (g *Game) drawUI() {
	g.buttons = g.buttons[:0]

	// User Info
	if g.state == StateMap {
		info := fmt.Sprintf("User: %s (Best: %d)", g.currentUser.Name, g.currentUser.BestScore)
		rl.DrawText(info, 10, 20, 20, getRlColor(colAccent))

		g.addButton(screenWidth-110, 10, 100, 30, "LEADERBOARD", func() {
			g.refreshLeaderboard()
			g.state = StateLeaderboard
		}, getRlColor(colGlass))
		g.addButton(screenWidth-220, 10, 80, 30, "LOGOUT", func() {
			g.state = StateLogin
			g.inputText = ""
		}, getRlColor(colDanger))
	}

	// Sidebar
	if g.selectedPlane != nil {
		panelW := 300
		panelX := screenWidth - panelW - 20
		g.drawPanel(panelX, 90, panelW, 350, "FLIGHT INFO")

		p := g.selectedPlane
		y := 140
		txtX := panelX + 20

		rl.DrawText(p.Callsign, int32(txtX), int32(y), 20, getRlColor(colAccent))
		y += 30
		rl.DrawText(fmt.Sprintf("Alt: %d ft", p.AltitudeFt), int32(txtX), int32(y), 16, rl.White)
		y += 25
		rl.DrawText(fmt.Sprintf("Spd: %d kts", p.VelocityKts), int32(txtX), int32(y), 16, rl.White)
		y += 25
		rl.DrawText(fmt.Sprintf("Pos: %.2f, %.2f", p.Lat, p.Lon), int32(txtX), int32(y), 16, rl.White)
		y += 35

		if g.resolving {
			rl.DrawText("Fetching details...", int32(txtX), int32(y), 16, getRlColor(colTextMuted))
		} else if g.resolvedDetails != nil {
			rl.DrawText("Model: "+truncate(g.resolvedDetails.Model, 25), int32(txtX), int32(y), 16, rl.White)
			y += 25

			orig := g.resolvedDetails.Origin
			dest := g.resolvedDetails.RealDestination

			if g.state == StateGamePlaying && g.targetPlane != nil && g.selectedPlane.Icao24 == g.targetPlane.Icao24 {
				if g.correctOption == orig {
					orig = "???"
				}
				if g.correctOption == dest {
					dest = "???"
				}
			}

			rl.DrawText("From: "+truncate(orig, 25), int32(txtX), int32(y), 16, rl.White)
			y += 25
			rl.DrawText("To:   "+truncate(dest, 25), int32(txtX), int32(y), 16, rl.White)
		} else {
			rl.DrawText("Details unavailable", int32(txtX), int32(y), 16, getRlColor(colTextMuted))
		}

		g.addButton(screenWidth-50, 95, 30, 30, "X", func() { g.selectedPlane = nil }, rl.Color{255, 255, 255, 50}, rl.Black)
	}

	// Game Panel
	if g.state == StateRoundSetup {
		g.drawPanel(20, 90, 300, 150, fmt.Sprintf("ROUND %d/5", g.round))
		rl.DrawText("Tracking target...", 40, 140, 20, rl.White)
	} else if g.state == StateGamePlaying && g.targetPlane != nil {
		// Increased height from 340 to 400 to fit score
		g.drawPanel(20, 90, 300, 400, fmt.Sprintf("ROUND %d/5", g.round))

		qText := g.questionText
		if len(qText) > 30 {
			qText = qText[:30] + "..."
		}
		rl.DrawText(qText, 30, 140, 20, rl.White)

		y := 180
		for _, opt := range g.options {
			col := rl.Fade(rl.White, 0.2)
			if g.showResult {
				if opt == g.correctOption {
					col = getRlColor(colSuccess)
				} else if !g.resultCorrect && opt == g.wrongGuess {
					col = getRlColor(colDanger)
				}
			}

			// Capture
			o := opt
			// Reduced height to 35, wider width 280
			g.addButton(30, y, 280, 35, truncate(o, 32), func() { g.guess(o) }, col, rl.Black)
			y += 45
		}

		rl.DrawText(fmt.Sprintf("Score: %d", g.score), 30, int32(y)+10, 20, getRlColor(colAccent))
		g.addButton(20, 450, 100, 30, "QUIT", func() { g.endGame() }, getRlColor(colDanger))
	}

	// Bottom Controls
	// Show PLAY GAME only if in Map mode
	if g.state == StateMap {
		g.addButton(screenWidth/2-60, screenHeight-60, 120, 40, "PLAY GAME", func() { g.startGame() }, getRlColor(colAccent))
		g.addButton(20, screenHeight-60, 80, 40, "CENTER", func() { g.camLat, g.camLon = myLat, myLon }, getRlColor(colGlass))
	}

	// Zoom buttons (Always show in Map AND GamePlaying)
	if g.state == StateMap || g.state == StateGamePlaying {
		g.addButton(screenWidth-110, screenHeight-60, 40, 40, "-", func() {
			if g.camZoom > 4 {
				g.camZoom--
			}
		}, getRlColor(colGlass))
		g.addButton(screenWidth-60, screenHeight-60, 40, 40, "+", func() {
			if g.camZoom < 18 {
				g.camZoom++
			}
		}, getRlColor(colGlass))
	}

	if g.state == StateGameOver {
		g.drawPanel(screenWidth/2-150, screenHeight/2-100, 300, 200, "GAME OVER")
		rl.DrawText(fmt.Sprintf("Final Score: %d", g.score), int32(screenWidth)/2-50, int32(screenHeight)/2, 20, rl.White)
		g.addButton(screenWidth/2-60, screenHeight/2+40, 120, 40, "CLOSE", func() { g.endGame() }, getRlColor(colAccent))
	}

	// Draw Buttons
	for _, b := range g.buttons {
		rl.DrawRectangle(int32(b.X), int32(b.Y), int32(b.W), int32(b.H), b.Color)
		// Centering text is a bit harder in Raylib without measuring,
		// but simple approx or MeasureText works.
		// Use font size 16 instead of 20 for smaller button text
		fontSize := int32(16)
		tw := rl.MeasureText(b.Text, fontSize)
		tx := b.X + (b.W-int(tw))/2
		ty := b.Y + (b.H-int(fontSize))/2 + 2
		rl.DrawText(b.Text, int32(tx), int32(ty), fontSize, b.TextColor)
	}
}

func (g *Game) drawPanel(x, y, w, h int, title string) {
	rl.DrawRectangle(int32(x), int32(y), int32(w), int32(h), getRlColor(colGlass))
	rl.DrawText(title, int32(x)+20, int32(y)+20, 20, getRlColor(colAccent))
}

func (g *Game) drawLogin() {
	g.buttons = g.buttons[:0]

	// DO NOT CHANGE THIS TITLE
	title := "VANTAA FLIGHTRADAR24"
	tw := rl.MeasureText(title, 30)
	rl.DrawText(title, int32(screenWidth-int(tw))/2, 80, 30, getRlColor(colAccent))

	if g.showDeleteConfirm {
		// Dialog
		panelX, panelY := screenWidth/2-150, 200
		rl.DrawRectangle(int32(panelX), int32(panelY), 300, 150, getRlColor(colGlass))
		rl.DrawText(fmt.Sprintf("Delete '%s'?", g.userToDelete), int32(panelX)+20, int32(panelY)+40, 20, rl.White)

		g.addButton(panelX+20, panelY+90, 100, 30, "CANCEL", func() { g.showDeleteConfirm = false }, getRlColor(colGlassLight))
		g.addButton(panelX+140, panelY+90, 100, 30, "DELETE", func() {
			g.dataManager.DeleteUser(g.userToDelete)
			g.refreshUsers()
			g.showDeleteConfirm = false
		}, getRlColor(colDanger))
	} else {
		// Input
		rl.DrawText("Select User or Type Name:", int32(screenWidth)/2-100, 160, 20, rl.White)
		rl.DrawRectangle(int32(screenWidth)/2-100, 180, 200, 30, rl.White)
		rl.DrawText(g.inputText, int32(screenWidth)/2-95, 185, 20, rl.Black)

		// Invisible button to toggle keyboard
		g.addButton(screenWidth/2-100, 180, 200, 30, "", func() { g.isKeyboardOpen = !g.isKeyboardOpen }, rl.Fade(rl.White, 0.0))

		if g.isKeyboardOpen {
			kbW, kbH := 520, 250
			kbX, kbY := (screenWidth-kbW)/2, 225
			rl.DrawRectangle(int32(kbX-10), int32(kbY-10), int32(kbW+20), int32(kbH+20), getRlColor(colBgDark))

			for r, row := range g.keyboardLayout {
				rowW := len(row) * 50
				rowStart := kbX + (kbW-rowW)/2
				for c, char := range row {
					charStr := string(char)
					bx := rowStart + c*50
					by := kbY + r*50
					g.addButton(bx, by, 45, 45, charStr, func() { g.inputText += charStr }, getRlColor(colGlassLight))
				}
			}

			ctrlY := kbY + 3*50 + 10
			g.addButton(kbX, ctrlY, 100, 45, "HIDE", func() { g.isKeyboardOpen = false }, getRlColor(colGlass))
			g.addButton(kbX+120, ctrlY, 100, 45, "DEL", func() {
				if len(g.inputText) > 0 {
					g.inputText = g.inputText[:len(g.inputText)-1]
				}
			}, getRlColor(colDanger))
			g.addButton(kbX+kbW-120, ctrlY, 120, 45, "ENTER", func() { g.login(g.inputText) }, getRlColor(colSuccess))
		} else {
			// User List
			y := 240
			var keys []string
			for k := range g.usersMap {
				keys = append(keys, k)
			}
			sort.Strings(keys)

			for _, name := range keys {
				u := g.usersMap[name]
				n := name
				label := fmt.Sprintf("%s (%d)", u.Name, u.BestScore)

				g.addButton(screenWidth/2-100, y, 200, 30, label, func() { g.login(n) }, getRlColor(colGlassLight))
				g.addButton(screenWidth/2+110, y, 30, 30, "X", func() {
					g.userToDelete = n
					g.showDeleteConfirm = true
				}, getRlColor(colDanger))
				y += 40
			}
		}
	}

	g.addButton(20, screenHeight-50, 100, 30, "QUIT", func() { g.shouldQuit = true }, getRlColor(colDanger))

	// Draw Buttons
	for _, b := range g.buttons {
		rl.DrawRectangle(int32(b.X), int32(b.Y), int32(b.W), int32(b.H), b.Color)
		// Centering text is a bit harder in Raylib without measuring,
		// but simple approx or MeasureText works.
		// Use font size 16 instead of 20 for smaller button text
		fontSize := int32(16)
		tw := rl.MeasureText(b.Text, fontSize)
		tx := b.X + (b.W-int(tw))/2
		ty := b.Y + (b.H-int(fontSize))/2 + 2
		rl.DrawText(b.Text, int32(tx), int32(ty), fontSize, b.TextColor)
	}
}

func (g *Game) drawLeaderboard() {
	g.buttons = g.buttons[:0]
	rl.DrawText("LEADERBOARD", 20, 30, 20, getRlColor(colAccent))

	rl.DrawText("TOP SCORES", 50, 70, 20, rl.White)
	y := 100
	for i, s := range g.highScores {
		line := fmt.Sprintf("%d. %s - %d", i+1, s.Name, s.Score)
		rl.DrawText(line, 50, int32(y), 20, rl.White)
		y += 25
	}

	rl.DrawText("PLAYER STATS", 400, 70, 20, rl.White)
	y = 100
	for i, u := range g.userStatsList {
		if i >= 10 {
			break
		}
		line := fmt.Sprintf("%s: Best %d | Played %d", u.Name, u.BestScore, u.GamesPlayed)
		rl.DrawText(line, 400, int32(y), 20, rl.White)
		y += 25
	}

	g.addButton(20, screenHeight-50, 100, 30, "BACK", func() { g.state = StateMap }, getRlColor(colDanger))

	for _, b := range g.buttons {
		rl.DrawRectangle(int32(b.X), int32(b.Y), int32(b.W), int32(b.H), b.Color)
		tw := rl.MeasureText(b.Text, 20)
		tx := b.X + (b.W-int(tw))/2
		ty := b.Y + (b.H-20)/2 + 2
		rl.DrawText(b.Text, int32(tx), int32(ty), 20, b.TextColor)
	}
}

func (g *Game) addButton(x, y, w, h int, label string, action func(), col rl.Color, txtCol ...rl.Color) {
	tc := rl.White
	if len(txtCol) > 0 {
		tc = txtCol[0]
	}
	g.buttons = append(g.buttons, Button{X: x, Y: y, W: w, H: h, Text: label, Action: action, Color: col, TextColor: tc})
}

// Helper methods from original (startGame, endGame, etc) need to be ported too
func (g *Game) startGame() {
	if len(g.flights) == 0 {
		return
	}
	g.score = 0
	g.round = 0
	g.nextRound()
}

func (g *Game) endGame() {
	if g.round > 0 {
		u, err := g.dataManager.SaveUser(g.currentUser.Name, g.score)
		if err == nil {
			g.currentUser = u
			g.usersMap[u.Name] = u
		}
		g.dataManager.AddScore(ScoreEntry{Name: g.currentUser.Name, Score: g.score, Date: time.Now().Format("2006-01-02")})
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
			g.pickNewTarget()
		}
	}()
}

func (g *Game) setupRoundWithData(details *ResolvedDetails) {
	g.resolvedDetails = details
	g.resolving = false
	if details.RealDestination == "" || details.RealDestination == "Unknown" {
		g.pickNewTarget()
		return
	}

	g.dataManager.SaveAirport(details.RealDestination)
	g.dataManager.SaveAirport(details.Origin)

	if strings.Contains(details.RealDestination, "Helsinki") {
		g.questionText = fmt.Sprintf("Where is %s from?", g.targetPlane.Callsign)
		g.correctOption = details.Origin
	} else {
		g.questionText = fmt.Sprintf("Where is %s going?", g.targetPlane.Callsign)
		g.correctOption = details.RealDestination
	}

	g.generateOptions()
	g.roundStartTime = time.Now()
	g.state = StateGamePlaying
}

func (g *Game) generateOptions() {
	g.refreshAirports()
	dist := make([]string, len(g.airports))
	copy(dist, g.airports)
	rand.Shuffle(len(dist), func(i, j int) { dist[i], dist[j] = dist[j], dist[i] })

	opts := []string{g.correctOption}
	for _, c := range dist {
		if len(opts) >= 4 {
			break
		}
		if c != g.correctOption && c != "Unknown" {
			opts = append(opts, c)
		}
	}
	if len(opts) < 4 {
		fallback := []string{"London", "Paris", "Berlin", "Helsinki"}
		for _, c := range fallback {
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
	rand.Shuffle(len(opts), func(i, j int) { opts[i], opts[j] = opts[j], opts[i] })
	g.options = opts
}

func (g *Game) guess(city string) {
	if g.showResult {
		return
	}
	g.resultCorrect = (city == g.correctOption)
	if g.resultCorrect {
		elapsed := time.Since(g.roundStartTime).Seconds()
		bonus := int(math.Max(0, (20.0-elapsed)/20.0*100.0))
		g.score += 100 + bonus
	} else {
		g.wrongGuess = city
	}
	g.showResult = true
	g.resultStartTime = time.Now()
}

func truncate(s string, max int) string {
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
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

	rl.InitWindow(screenWidth, screenHeight, "Flight Monitor Raylib")
	rl.SetTargetFPS(60)

	client := NewFlightClient()
	game := NewGame(client)
	game.Init()
	defer game.Unload()

	for !rl.WindowShouldClose() && !game.shouldQuit {
		game.Update()
		game.Draw()
	}

	rl.CloseWindow()
}
