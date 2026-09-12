import os
import socket
import time

socket_path = os.environ["DOGSTATSD_SOCKET"]
mode = os.environ["TELEMETRY_MODE"]
tags = "client:go,client_version:e2e,client_transport:uds"
if mode not in {"unhealthy", "healthy"}:
    raise ValueError(f"unsupported telemetry mode: {mode}")

client = socket.socket(socket.AF_UNIX, socket.SOCK_DGRAM)
metric = b"synthetic.dogstatsd.client.drop:1|c"

while True:
    sent = 0
    dropped = 0

    if mode == "unhealthy":
        for _ in range(10):
            try:
                client.sendto(metric, socket_path + ".missing")
                sent += len(metric)
            except FileNotFoundError:
                dropped += len(metric)

    client.sendto(metric, socket_path)
    sent += len(metric)

    values = {
        "bytes_sent": sent,
        "bytes_dropped": dropped,
        "bytes_dropped_queue": 0,
        "bytes_dropped_writer": dropped,
    }
    telemetry = "\n".join(
        f"datadog.dogstatsd.client.{name}:{value}|c|#{tags}" for name, value in values.items()
    ).encode()
    client.sendto(telemetry, socket_path)
    time.sleep(1)
