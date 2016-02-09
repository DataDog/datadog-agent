import requests

def retrieve_json(url):
    r = requests.get(url)
    r.raise_for_status()
    return r.json()
