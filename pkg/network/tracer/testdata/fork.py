#!/usr/bin/env python3

import os
import signal
import socket

s = socket.socket(family=socket.AF_INET, type=socket.SOCK_STREAM)
s.bind(("localhost", 0))
addr = s.getsockname()
print(addr[1])
s.connect(addr)

pid = os.fork()
if pid == 0:
    # child
    s.send(b'foo')
else:
    # parent
    s.recv(256)
    os.wait()
    signal.pause()

s.close()
