# Overhead Flights Monitor

A Raspberry Pi flight tracker with a premium touchscreen UI and a "Guess the Destination" game.

## Features
- **Live Tracking**: Real-time flight positions using FlightRadarAPI.
- **Premium UI**: Dark mode, glassmorphism, and smooth animations.
- **Game Mode**: Guess where planes are heading to earn points.
- **Touchscreen Ready**: Large buttons and on-screen keyboard.
- **Autostart**: Runs automatically on boot.

## Installation

1. **Clone the repo:**
   ```bash
   git clone https://github.com/yourusername/overhead_flights_monitor.git
   cd overhead_flights_monitor
   ```

2. **Install dependencies:**
   ```bash
   python3 -m venv .venv
   source .venv/bin/activate
   pip install flask FlightRadarAPI beautifulsoup4
   ```

3. **Run manually:**
   ```bash
   python server.py
   ```
   Open `http://localhost:4000` in your browser.

## Autostart on Raspberry Pi

1. Edit `flight-monitor.service` and check paths:
   ```ini
   WorkingDirectory=/home/pi/overhead_flights_monitor
   ExecStart=/home/pi/overhead_flights_monitor/.venv/bin/python server.py
   ```

2. Copy service file:
   ```bash
   sudo cp flight-monitor.service /etc/systemd/system/
   ```

3. Enable and start:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable flight-monitor.service
   sudo systemctl start flight-monitor.service
   ```

## Configuration
Edit `server.py` or set env vars `MY_LAT` and `MY_LON` to change your location.
