from FlightRadar24 import FlightRadar24API
fr_api = FlightRadar24API()
airport = fr_api.get_airport("IND")
print(vars(airport))
