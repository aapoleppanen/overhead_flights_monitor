package main

import (
	"fmt"
	"image"
	"net/http"
	"sync"

	"github.com/hajimehoshi/ebiten/v2"
)

type TileKey struct {
	Z, X, Y int
}

type TileLoader struct {
	cache      map[TileKey]*ebiten.Image
	mutex      sync.Mutex
	httpClient *http.Client
}

func NewTileLoader() *TileLoader {
	return &TileLoader{
		cache:      make(map[TileKey]*ebiten.Image),
		httpClient: &http.Client{},
	}
}

func (tl *TileLoader) GetTile(z, x, y int) *ebiten.Image {
	key := TileKey{z, x, y}

	tl.mutex.Lock()
	if img, ok := tl.cache[key]; ok {
		tl.mutex.Unlock()
		return img
	}
	tl.mutex.Unlock()

	// If not in cache, return nil (or a placeholder) and fetch in background
	go tl.fetchTile(z, x, y)
	return nil
}

func (tl *TileLoader) fetchTile(z, x, y int) {
	// Check cache again before fetching
	tl.mutex.Lock()
	if _, ok := tl.cache[TileKey{z, x, y}]; ok {
		tl.mutex.Unlock()
		return
	}
	tl.mutex.Unlock()

	// CartoDB Dark Matter URL
	url := fmt.Sprintf("https://basemaps.cartocdn.com/dark_all/%d/%d/%d.png", z, x, y)

	resp, err := tl.httpClient.Get(url)
	if err != nil {
		fmt.Println("Failed to fetch tile:", err)
		return
	}
	defer resp.Body.Close()

	img, _, err := image.Decode(resp.Body)
	if err != nil {
		fmt.Println("Failed to decode tile:", err)
		return
	}

	ebitenImg := ebiten.NewImageFromImage(img)

	tl.mutex.Lock()
	tl.cache[TileKey{z, x, y}] = ebitenImg
	tl.mutex.Unlock()
}
