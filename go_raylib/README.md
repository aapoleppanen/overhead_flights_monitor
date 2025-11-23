# Flight Monitor (Raylib Version)

This is a port of the Overhead Flight Monitor to Raylib for better performance on Raspberry Pi 3 (OpenGL ES 2.0).

## Prerequisites (Raspberry Pi / Linux)

You need the Raylib dependencies:

```bash
sudo apt update
sudo apt install libgl1-mesa-dev libxi-dev libxcursor-dev libxrandr-dev libxinerama-dev libwayland-dev libxkbcommon-dev
```

## Build & Run

```bash
cd go_raylib
go mod tidy
go build -o flight-monitor-raylib .
./flight-monitor-raylib
```

## Configuration

The same environment variables apply:
- `MY_LAT`: Your latitude
- `MY_LON`: Your longitude
- `CLIENT_ID`: OpenSky Username (optional)
- `CLIENT_SECRET`: OpenSky Password (optional)

## Controls
- **Touch**: Drag to pan, Pinch to zoom (requires multi-touch support in OS).
- **Mouse**: Click-drag to pan, Scroll to zoom.
- **Keyboard**: On-screen keyboard for login.
