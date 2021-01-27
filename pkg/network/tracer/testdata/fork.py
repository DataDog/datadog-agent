#!/usr/bin/env python3

import os
import signal
import socket

s = socket.socket(family=socket.AF_INET, type=socket.SOCK_STREAM)
s.bind(("localhost", 33333))
s.connect(("localhost", 33333))

pid = os.fork()
if pid == 0:
    # child
    s.send(b'foo')
else:
    # parent
    s.recv(256)
    os.wait()
    signal.sigwait([signal.SIGKILL, signal.SIGINT, signal.SIGSTOP])

s.close()
