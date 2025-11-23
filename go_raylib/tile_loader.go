package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"

	rl "github.com/gen2brain/raylib-go/raylib"
)

type TileKey struct {
	Z, X, Y int
}

type TileResponse struct {
	Key  TileKey
	Data []byte
}

type TileLoader struct {
	cache        map[TileKey]rl.Texture2D
	pending      map[TileKey]bool
	responseChan chan TileResponse
	mutex        sync.Mutex
	httpClient   *http.Client
}

func NewTileLoader() *TileLoader {
	return &TileLoader{
		cache:        make(map[TileKey]rl.Texture2D),
		pending:      make(map[TileKey]bool),
		responseChan: make(chan TileResponse, 10), // Buffer slightly
		httpClient:   &http.Client{},
	}
}

// GetTile returns the texture if available. Returns empty texture (id=0) if not.
// It triggers a fetch if not already cached or pending.
func (tl *TileLoader) GetTile(z, x, y int) rl.Texture2D {
	key := TileKey{z, x, y}

	// 1. Check Cache
	// Note: Maps are not safe for concurrent R/W, but we mainly access on main thread here.
	// However, fetching is async. The cache writing happens in Update() which is main thread.
	// So reading here is safe if GetTile is called from main thread.
	if tex, ok := tl.cache[key]; ok {
		return tex
	}

	// 2. Check Pending
	tl.mutex.Lock()
	if tl.pending[key] {
		tl.mutex.Unlock()
		return rl.Texture2D{} // Return empty/invalid texture
	}
	tl.pending[key] = true
	tl.mutex.Unlock()

	// 3. Start Fetch
	go tl.fetchTile(z, x, y)

	return rl.Texture2D{}
}

// Update processes loaded images and uploads them to GPU. Must call on Main Thread.
func (tl *TileLoader) Update() {
	// Drain the channel up to a limit to avoid stuttering? Or just all.
	// Let's do all available.
Loop:
	for {
		select {
		case resp := <-tl.responseChan:
			// Load Image from RAM
			// ".png" is the file type hint
			img := rl.LoadImageFromMemory(".png", resp.Data, int32(len(resp.Data)))
			if img.Width == 0 {
				fmt.Println("Failed to load image from memory for tile", resp.Key)
				// Clean up pending so we might retry later? Or just leave it broken.
				continue
			}

			// Upload to GPU
			tex := rl.LoadTextureFromImage(img)

			// Free CPU RAM
			rl.UnloadImage(img)

			// Store in cache
			tl.cache[resp.Key] = tex

			// Cleanup pending (optional, but good for logic)
			tl.mutex.Lock()
			delete(tl.pending, resp.Key)
			tl.mutex.Unlock()

		default:
			break Loop
		}
	}
}

func (tl *TileLoader) fetchTile(z, x, y int) {
	key := TileKey{z, x, y}
	url := fmt.Sprintf("https://basemaps.cartocdn.com/dark_all/%d/%d/%d.png", z, x, y)

	resp, err := tl.httpClient.Get(url)
	if err != nil {
		fmt.Println("Failed to fetch tile:", err)
		tl.mutex.Lock()
		delete(tl.pending, key)
		tl.mutex.Unlock()
		return
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Failed to read tile body:", err)
		tl.mutex.Lock()
		delete(tl.pending, key)
		tl.mutex.Unlock()
		return
	}

	// Send to main thread
	tl.responseChan <- TileResponse{Key: key, Data: data}
}

// Unload cleans up all textures
func (tl *TileLoader) Unload() {
	for _, tex := range tl.cache {
		rl.UnloadTexture(tex)
	}
	tl.cache = make(map[TileKey]rl.Texture2D)
}
