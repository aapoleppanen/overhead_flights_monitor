from FlightRadar24 import FlightRadar24API
fr_api = FlightRadar24API()
flights = fr_api.get_flights()

# Find a flight with destination
target = None
for f in flights:
    if f.destination_airport_iata and f.destination_airport_iata != 'N/A':
        target = f
        break

if target:
    print(f"Found flight to {target.destination_airport_iata}")
    # Get airport details
    airport = fr_api.get_airport(target.destination_airport_iata)
    print("Airport details:")
    print(airport)
else:
    print("No flights with destination found.")
