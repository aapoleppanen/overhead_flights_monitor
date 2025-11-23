from FlightRadar24 import FlightRadar24API
fr_api = FlightRadar24API()
flights = fr_api.get_flights()
print(f"Found {len(flights)} flights")
if flights:
    f = flights[0]
    details = fr_api.get_flight_details(f)
    print(details)
