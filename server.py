import os
import json
from flask import Flask, render_template, jsonify
import requests
import math

# --- Configuration ---
# Vantaa, Finland as an example.
# REPLACE WITH YOUR LOCATION!
MY_LAT = float(os.getenv("MY_LAT", "60.3172"))
MY_LON = float(os.getenv("MY_LON", "24.9633"))
# Bounding box area in degrees (1 degree lat ~= 111km)
# This creates a box roughly 111km tall x 55km wide
AREA_RADIUS_DEG = 0.5

# Calculate the bounding box for the OpenSky Network API
BBOX = [
    MY_LAT - AREA_RADIUS_DEG,  # lat_min
    MY_LAT + AREA_RADIUS_DEG,  # lat_max
    MY_LON - (AREA_RADIUS_DEG / math.cos(math.radians(MY_LAT))), # lon_min
    MY_LON + (AREA_RADIUS_DEG / math.cos(math.radians(MY_LAT))), # lon_max
]

OPENSKY_URL = (
    "https://opensky-network.org/api/states/all?"
    f"lamin={BBOX[0]}&lomin={BBOX[2]}&lamax={BBOX[1]}&lomax={BBOX[3]}"
)

app = Flask(__name__)

def get_flight_data():
    """
    Fetches flight data from the OpenSky Network.
    """
    planes_list = []
    try:
        # OpenSky is public, no auth needed
        res = requests.get(OPENSKY_URL, timeout=10)
        res.raise_for_status() # Raise an error for bad responses (4xx or 5xx)

        data = res.json()

        if data['states']:
            for state in data['states']:
                # OpenSky state vector indices:
                # 0: icao24, 1: callsign, 2: origin_country, 5: longitude, 6: latitude,
                # 8: on_ground, 9: velocity, 10: true_track (heading), 11: vertical_rate,
                # 13: geo_altitude

                # We only want planes that are in the air
                if not state[8]: # state[8] is 'on_ground'
                    plane = {
                        "icao24": state[0],
                        "callsign": state[1].strip() if state[1] else "N/A",
                        "lon": state[5],
                        "lat": state[6],
                        "velocity_ms": state[9],
                        "heading": state[10],
                        "altitude_m": state[13]
                    }
                    planes_list.append(plane)

    except requests.exceptions.RequestException as e:
        print(f"Error fetching from OpenSky: {e}")
        # In a real app, you might return an error message
    except json.JSONDecodeError:
        print("Error: Could not decode JSON response from OpenSky.")
    except Exception as e:
        print(f"An unexpected error occurred: {e}")

    return planes_list

@app.route('/')
def index():
    """
    Serves the main HTML page.
    """
    # Pass the location config to the frontend
    return render_template('index.html', my_lat=MY_LAT, my_lon=MY_LON)

@app.route('/api/flights')
def api_flights():
    """
    This is the local API our frontend will call.
    """
    flights = get_flight_data()
    return jsonify(flights)

if __name__ == '__main__':
    # Run the server on port 5000, accessible from any device on the network
    # On the Pi, you'll access this at http://localhost:5000
    app.run(host='0.0.0.0', port=4000)
