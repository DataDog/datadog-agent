import datadog
import time

client = datadog.dogstatsd.base.DogStatsd(socket_path="/dsd.socket")

# Send 10 packets
for x in xrange(1, 10):
    client.increment('custom_counter1')
    time.sleep(0.25)

# Wait until killed
while True:
    time.sleep(1)
