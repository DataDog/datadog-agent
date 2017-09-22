import datadog
import time

client = datadog.dogstatsd.base.DogStatsd(socket_path="/dsd.socket")

# Send 4 packets/s until killed
while True:
    client.increment('custom_counter1')
    time.sleep(0.25)
