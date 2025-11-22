import os
import json
from flask import Flask, render_template, jsonify, request
from FlightRadar24 import FlightRadar24API
import math

# --- Configuration ---
# Vantaa, Finland as an example.
# REPLACE WITH YOUR LOCATION!
MY_LAT = float(os.getenv("MY_LAT", "60.3172"))
MY_LON = float(os.getenv("MY_LON", "24.9633"))
# Bounding box area in degrees (1 degree lat ~= 111km)
AREA_RADIUS_DEG = 1.0 # Increased radius for more flights

# Calculate the bounding box for FlightRadar24 (North, South, West, East)
# Note: FR24 uses "y1,y2,x1,x2" -> "lat_max, lat_min, lon_min, lon_max" ?
# Actually the library `get_flights` usually takes bounds as a string "ymax,ymin,xmin,xmax"
BBOX_STR = f"{MY_LAT + AREA_RADIUS_DEG},{MY_LAT - AREA_RADIUS_DEG},{MY_LON - AREA_RADIUS_DEG},{MY_LON + AREA_RADIUS_DEG}"

SCORES_FILE = "scores.json"

app = Flask(__name__)
fr_api = FlightRadar24API()

# Cache for airport IATA -> City Name
# This prevents us from spamming the API for the same airport details
AIRPORT_CACHE = {}

def get_airport_city(iata):
    """
    Resolves an IATA code to a city name using FlightRadarAPI + Caching.
    """
    if not iata or iata == 'N/A':
        return None
    
    if iata in AIRPORT_CACHE:
        return AIRPORT_CACHE[iata]
    
    try:
        airport = fr_api.get_airport(iata)
        if airport and 'city' in airport:
            city = airport['city']
            AIRPORT_CACHE[iata] = city
            return city
    except Exception as e:
        print(f"Error fetching airport {iata}: {e}")
    
    return None

def get_flight_data():
    """
    Fetches flight data from FlightRadar24.
    """
    planes_list = []
    try:
        # Fetch flights within bounds
        flights = fr_api.get_flights(bounds=BBOX_STR)
        
        for f in flights:
            # We only want planes that are in the air (on_ground is 0)
            if not f.on_ground:
                
                # Try to get destination city
                dest_city = None
                if f.destination_airport_iata and f.destination_airport_iata != 'N/A':
                    dest_city = get_airport_city(f.destination_airport_iata)

                plane = {
                    "id": f.id,
                    "icao24": f.icao_24bit,
                    "callsign": f.callsign,
                    "lon": f.longitude,
                    "lat": f.latitude,
                    "velocity_kts": f.ground_speed, # FR24 gives knots
                    "heading": f.heading,
                    "altitude_ft": f.altitude, # FR24 gives feet
                    "destination": dest_city,
                    "destination_iata": f.destination_airport_iata
                }
                planes_list.append(plane)

    except Exception as e:
        print(f"Error fetching from FlightRadar24: {e}")

    return planes_list

@app.route('/')
def index():
    """
    Serves the main HTML page.
    """
    return render_template('index.html', my_lat=MY_LAT, my_lon=MY_LON)

@app.route('/api/flights')
def api_flights():
    """
    This is the local API our frontend will call.
    """
    flights = get_flight_data()
    return jsonify(flights)

@app.route('/api/scores', methods=['GET', 'POST'])
def api_scores():
    """
    Handle score saving and loading.
    """
    if request.method == 'GET':
        try:
            if os.path.exists(SCORES_FILE):
                with open(SCORES_FILE, 'r') as f:
                    return jsonify(json.load(f))
            return jsonify([])
        except Exception as e:
            return jsonify({"error": str(e)}), 500

    if request.method == 'POST':
        try:
            new_score = request.json
            scores = []
            if os.path.exists(SCORES_FILE):
                with open(SCORES_FILE, 'r') as f:
                    try:
                        scores = json.load(f)
                    except json.JSONDecodeError:
                        scores = []
            
            scores.append(new_score)
            # Keep top 10 scores
            scores.sort(key=lambda x: x.get('score', 0), reverse=True)
            scores = scores[:10]
            
            with open(SCORES_FILE, 'w') as f:
                json.dump(scores, f)
                
            return jsonify({"success": True, "scores": scores})
        except Exception as e:
            return jsonify({"error": str(e)}), 500

if __name__ == '__main__':
    # Run the server on port 5000, accessible from any device on the network
    # On the Pi, you'll access this at http://localhost:5000
    app.run(host='0.0.0.0', port=4000)
