import time

import datadog

client = datadog.dogstatsd.base.DogStatsd(socket_path="/tmp/scratch/dsd.socket")

# Send 4 packets/s until killed
while True:
    client.increment('custom_counter1')
    time.sleep(0.25)
