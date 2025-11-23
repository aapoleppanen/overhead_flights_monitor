# Flight Monitor (Go Version)

This is a high-performance native version of the Flight Monitor, rewritten in Go using the [Ebitengine](https://ebitengine.org/) game engine. This version should run significantly smoother on Raspberry Pi 3 hardware.

## Prerequisites

1.  **Install Go**:
    ```bash
    sudo apt update
    sudo apt install golang -y
    ```
    (Or download the latest version from [go.dev](https://go.dev/dl/) if the apt version is too old. You need Go 1.18+).

2.  **Install Graphics Dependencies** (for Ebitengine on Linux/Pi):
    ```bash
    sudo apt install libgl1-mesa-dev libxcursor-dev libxi-dev libxinerama-dev libxrandr-dev libxxf86vm-dev libasound2-dev pkg-config
    ```

## Setup

1.  Navigate to this directory:
    ```bash
    cd go_version
    ```

2.  Download dependencies:
    ```bash
    go mod tidy
    ```

## Running the App

```bash
go run .
```

Or build a binary for faster startup:

```bash
go build -o flight-monitor
./flight-monitor
```

## Configuration

The app uses the same environment variables as the Python version:

```bash
export MY_LAT=60.3172
export MY_LON=24.9633
./flight-monitor
```

## Controls

*   **Arrow Keys**: Pan the map.
*   **+/- (or Mouse Wheel)**: Zoom in/out.

## Implementation Details

*   **Map**: Fetches "Dark Matter" tiles from CartoDB directly.
*   **Flights**: Polls OpenSky Network every 10 seconds.
*   **Rendering**: Uses GPU acceleration via Ebitengine.
