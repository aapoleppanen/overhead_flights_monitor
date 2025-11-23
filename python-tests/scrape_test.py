import re
import json
import sys
import os

def get_flight_details_from_file(filepath):
    try:
        print(f"Reading {filepath}...")
        with open(filepath, 'r', encoding='utf-8') as f:
            content = f.read()

        # Try to find the script block
        # Match "trackpollBootstrap = { ... };" handling potential whitespace and 'var'
        match = re.search(r'(?:var\s+)?trackpollBootstrap\s*=\s*({.+?});', content, re.DOTALL)

        if match:
            json_str = match.group(1)
            try:
                data = json.loads(json_str)
            except json.JSONDecodeError:
                print("JSON Decode Error")
                return None

            if "flights" in data:
                # The key is dynamic (e.g. "SWR22-1763624060...")
                # We just want the first one available
                for flight_id, flight_data in data["flights"].items():
                    print(f"Checking flight ID: {flight_id}")

                    # Check structure
                    if "activityLog" in flight_data and "flights" in flight_data["activityLog"]:
                        flights_log = flight_data["activityLog"]["flights"]
                        if not flights_log:
                            print("Empty flights log")
                            continue

                        latest_log = flights_log[0]

                        dest_data = latest_log.get("destination", {})
                        aircraft_data = latest_log.get("aircraft", {})

                        destination = dest_data.get("friendlyLocation") or dest_data.get("iata")
                        model = aircraft_data.get("friendlyType") or aircraft_data.get("type")

                        origin = latest_log.get("origin", {}).get("friendlyLocation")

                        print(f"Found: Origin: {origin} | Dest: {destination} | Model: {model}")
                        return {"destination": destination, "model": model, "origin": origin}

                print("No valid flight activity log found in data['flights']")
            else:
                print("Key 'flights' not found in data")

        else:
            print("Could not parse trackpollBootstrap from HTML")

        return None

    except Exception as e:
        print(f"Error: {e}")
        return None

if __name__ == "__main__":
    get_flight_details_from_file("example_fligtaware.html")
