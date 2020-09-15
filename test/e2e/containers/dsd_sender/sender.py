import time

import datadog

client = datadog.dogstatsd.base.DogStatsd(socket_path="/var/run/dogstatsd/dsd.socket")

while True:
    # Nominal case, dsd will inject its hostname
    client.gauge('dsd.hostname.e2e', 1, tags=["case:nominal"])
    client.service_check('dsd.hostname.e2e', 0, tags=["case:nominal"])
    client.event('dsd.hostname.e2e', 'text', tags=["case:nominal"])

    # Force the hostname value
    client.gauge('dsd.hostname.e2e', 1, tags=["case:forced", "host:forced"])
    client.service_check('dsd.hostname.e2e', 0, tags=["case:forced"], hostname="forced")
    client.event('dsd.hostname.e2e', 'text', tags=["case:forced"], hostname="forced")

    # Force an empty hostname
    client.gauge('dsd.hostname.e2e', 1, tags=["case:empty", "host:"])
    client.service_check('dsd.hostname.e2e', 0, tags=["case:empty", "host:"])
    client.event('dsd.hostname.e2e', 'text', tags=["case:empty", "host:"])

    time.sleep(10)
