import os
import socket
import time

import datadog

SOCKET_PATH = os.getenv("SOCKET_PATH", "/tmp/scratch/dsd.socket")


def send_dagagram():
    client = datadog.dogstatsd.base.DogStatsd(socket_path=SOCKET_PATH)

    # Send 4 packets/s until killed
    while True:
        client.increment('custom_counter1')
        time.sleep(0.25)


def send_stream():
    # TODO: switch to DogStatsdClient when it supports unix stream sockets
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.settimeout(5)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 32 * 1024)
    s.connect(SOCKET_PATH)

    # Send 4 packets/s until killed
    while True:
        msg = b"custom_counter1:1|c"
        s.send(len(msg).to_bytes(4, byteorder='little'))
        s.send(msg)
        time.sleep(0.25)


def main():
    socket_type = os.environ.get('SOCKET_TYPE', 'unixgram')
    print("Socket type: " + str(socket_type))
    if socket_type == 'unixgram':
        send_dagagram()
    elif socket_type == 'unix':
        send_stream()


if __name__ == '__main__':
    main()
