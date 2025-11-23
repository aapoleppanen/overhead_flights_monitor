import os
import json
import time
import requests
import re
from flask import Flask, render_template, jsonify, request

# --- Configuration ---
# Helsinki, Finland (User's location based on previous context)
MY_LAT = float(os.getenv("MY_LAT", "60.3172"))
MY_LON = float(os.getenv("MY_LON", "24.9633"))

# Bounding box radius in degrees (approx 1 deg ~ 111km)
AREA_RADIUS_DEG = 1.0

SCORES_FILE = "scores.json"
USERS_FILE = "users.json"

app = Flask(__name__)

# --- OpenSky API Constants ---
OPENSKY_API_URL = "https://opensky-network.org/api/states/all"

CATEGORY_MAP = {
    0: "No Info", 1: "No Info", 2: "Light (< 15.5k lbs)", 3: "Small (15.5k-75k lbs)",
    4: "Large (75k-300k lbs)", 5: "High Vortex Large", 6: "Heavy (> 300k lbs)",
    7: "High Performance", 8: "Rotorcraft", 9: "Glider", 10: "Lighter-than-air",
    11: "Parachutist", 12: "Ultralight", 13: "Reserved", 14: "UAV", 15: "Space Vehicle",
    16: "Emergency Vehicle", 17: "Service Vehicle", 18: "Point Obstacle", 19: "Cluster Obstacle",
    20: "Line Obstacle"
}

def infer_destination(lat, lon, heading):
    """
    Infers a destination city based on the heading from the current location.
    This is a fallback since OpenSky doesn't provide destination in state vectors.
    """
    # Simple directional logic relative to Helsinki/Vantaa (approx 60N, 25E)
    # This is for GAME PURPOSES ONLY.

    if heading is None:
        return None

    # Normalized heading 0-360
    h = heading % 360

    if 337.5 <= h or h < 22.5:
        return "Rovaniemi" # North
    elif 22.5 <= h < 67.5:
        return "Joensuu" # NE
    elif 67.5 <= h < 112.5:
        return "St. Petersburg" # East
    elif 112.5 <= h < 157.5:
        return "Moscow" # SE
    elif 157.5 <= h < 202.5:
        return "Tallinn" # South
    elif 202.5 <= h < 247.5:
        return "Berlin" # SW
    elif 247.5 <= h < 292.5:
        return "Stockholm" # West
    elif 292.5 <= h < 337.5:
        return "Tampere" # NW

    return "Unknown"

def fetch_flight_details(callsign):
    """
    Scrapes FlightAware for destination and aircraft model.
    """
    url = f"https://www.flightaware.com/live/flight/{callsign}"
    headers = {
        "User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
        "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
        "Accept-Language": "en-US,en;q=0.5",
    }

    try:
        print(f"Resolving {callsign} via FlightAware...")
        response = requests.get(url, headers=headers, timeout=10)
        response.raise_for_status()

        # Match "trackpollBootstrap = { ... };" handling potential whitespace and 'var'
        match = re.search(r'(?:var\s+)?trackpollBootstrap\s*=\s*({.+?});', response.text, re.DOTALL)

        if match:
            json_str = match.group(1)
            try:
                data = json.loads(json_str)
            except json.JSONDecodeError:
                return None

            if "flights" in data:
                for flight_id, flight_data in data["flights"].items():
                    if "activityLog" in flight_data and "flights" in flight_data["activityLog"]:
                        flights_log = flight_data["activityLog"]["flights"]
                        if not flights_log: continue

                        latest_log = flights_log[0]

                        dest_data = latest_log.get("destination", {})
                        aircraft_data = latest_log.get("aircraft", {})

                        destination = dest_data.get("friendlyLocation") or dest_data.get("iata")
                        model = aircraft_data.get("friendlyType") or aircraft_data.get("type")
                        origin = latest_log.get("origin", {}).get("friendlyLocation")

                        # Game Logic: If destination is local (Vantaa/Helsinki), use origin as the target
                        game_city = destination
                        origin_display = origin
                        if destination and ("Vantaa" in destination or "Helsinki" in destination):
                            game_city = origin
                            origin_display = "Hidden" # Hide origin if it becomes the answer

                        return {
                            "destination": game_city,
                            "real_destination": destination,
                            "model": model,
                            "origin": origin_display
                        }
    except Exception as e:
        print(f"Error resolving {callsign}: {e}")

    return None

def get_flight_data():
    """
    Fetches flight data from OpenSky Network REST API.
    Returns a list of flight dictionaries formatted for the frontend.
    """
    flights_list = []

    # Calculate bounding box
    lamin = MY_LAT - AREA_RADIUS_DEG
    lamax = MY_LAT + AREA_RADIUS_DEG
    lomin = MY_LON - AREA_RADIUS_DEG
    lomax = MY_LON + AREA_RADIUS_DEG

    params = {
        "lamin": lamin,
        "lomin": lomin,
        "lamax": lamax,
        "lomax": lomax,
        "extended": 1
    }

    try:
        response = requests.get(OPENSKY_API_URL, params=params, timeout=10)
        response.raise_for_status()
        data = response.json()

        if 'states' in data and data['states']:
            for s in data['states']:
                # Filter out incomplete data
                if s[5] is None or s[6] is None:
                    continue

                # Conversions
                altitude_m = s[7] if s[7] is not None else 0
                altitude_ft = int(altitude_m * 3.28084)

                velocity_ms = s[9] if s[9] is not None else 0
                velocity_kts = int(velocity_ms * 1.94384)

                callsign = s[1].strip() if s[1] else "N/A"

                # Map category
                if len(s) > 17:
                    cat_code = s[17] if s[17] is not None else 0
                    category_str = CATEGORY_MAP.get(cat_code, "Unknown")
                else:
                    category_str = "Unknown"

                flight = {
                    "id": s[0],             # icao24
                    "icao24": s[0],
                    "callsign": callsign,
                    "lon": s[5],
                    "lat": s[6],
                    "velocity_kts": velocity_kts,
                    "heading": s[10] if s[10] is not None else 0,
                    "altitude_ft": altitude_ft,
                    "destination": infer_destination(s[6], s[5], s[10]), # Fallback
                    "on_ground": s[8],
                    "origin_country": s[2],
                    "category": category_str
                }
                flights_list.append(flight)

    except requests.RequestException as e:
        print(f"Error fetching from OpenSky: {e}")
    except Exception as e:
        print(f"An error occurred: {e}")

    return flights_list

@app.route('/')
def index():
    """
    Serves the main HTML page.
    """
    return render_template('index.html', my_lat=MY_LAT, my_lon=MY_LON)

@app.route('/api/flights')
def api_flights():
    """
    API endpoint for the frontend to get flight data.
    """
    flights = get_flight_data()
    return jsonify(flights)

@app.route('/api/resolve/<callsign>')
def api_resolve(callsign):
    """
    Resolves flight details (Destination, Model) via FlightAware.
    """
    details = fetch_flight_details(callsign)
    if details:
        return jsonify(details)
    return jsonify({"error": "Could not resolve details"}), 404

@app.route('/api/users/<username>', methods=['GET', 'POST'])
def api_users(username):
    """
    Handle user stats (Win rate, Best score).
    """
    users_data = {}
    if os.path.exists(USERS_FILE):
        try:
            with open(USERS_FILE, 'r') as f:
                content = f.read()
                if content:
                    users_data = json.loads(content)
        except:
            users_data = {}

    if request.method == 'GET':
        user = users_data.get(username, {"name": username, "games_played": 0, "total_score": 0, "best_score": 0})
        return jsonify(user)

    if request.method == 'POST':
        # Update stats after a game
        data = request.json
        score = data.get('score', 0)

        user = users_data.get(username, {"name": username, "games_played": 0, "total_score": 0, "best_score": 0})
        user['games_played'] += 1
        user['total_score'] += score
        if score > user['best_score']:
            user['best_score'] = score

        users_data[username] = user

        with open(USERS_FILE, 'w') as f:
            json.dump(users_data, f)

        return jsonify(user)

@app.route('/api/scores', methods=['GET', 'POST'])
def api_scores():
    """
    Handle score saving and loading.
    Returns both global high scores and user statistics.
    """
    if request.method == 'GET':
        try:
            # 1. Global High Scores
            high_scores = []
            if os.path.exists(SCORES_FILE):
                with open(SCORES_FILE, 'r') as f:
                    content = f.read()
                    if content:
                        high_scores = json.loads(content)

            # 2. User Stats
            user_stats = []
            if os.path.exists(USERS_FILE):
                try:
                    with open(USERS_FILE, 'r') as f:
                        content = f.read()
                        if content:
                            users_dict = json.loads(content)
                            # Calculate performance stats
                            for u in users_dict.values():
                                games = u.get('games_played', 0)
                                total = u.get('total_score', 0)

                                # Assuming approx 1000 max points per game (200 * 5 rounds)
                                # This is an estimate for "historical percentage"
                                percentage = 0
                                if games > 0:
                                    # Max potential score is roughly 1000 per game
                                    percentage = int((total / (games * 1000)) * 100)
                                    percentage = min(100, max(0, percentage)) # Clamp 0-100

                                u['performance_percent'] = percentage
                                user_stats.append(u)

                            # Sort users by best score (descending)
                            user_stats.sort(key=lambda x: x.get('best_score', 0), reverse=True)
                except Exception as e:
                    print(f"Error reading user stats: {e}")

            return jsonify({
                "high_scores": high_scores,
                "user_stats": user_stats
            })
        except Exception as e:
            return jsonify({"error": str(e)}), 500

    if request.method == 'POST':
        try:
            new_score = request.json
            scores = []
            if os.path.exists(SCORES_FILE):
                with open(SCORES_FILE, 'r') as f:
                    try:
                        content = f.read()
                        if content:
                            scores = json.loads(content)
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
    # Run the server on port 4000
    # Turn off debug mode for better performance on Pi
    app.run(host='0.0.0.0', port=4000, debug=False)
