from FlightRadar24 import FlightRadar24API
fr_api = FlightRadar24API()
flights = fr_api.get_flights()
print(f"Found {len(flights)} flights")
if flights:
    f = flights[0]
    print("Flight attributes:")
    print(vars(f))
    
    # Try to get details
    details = fr_api.get_flight_details(f)
    print("\nFlight details:")
    print(details)
