#!/usr/bin/env python3

import os
import signal
import socket

s = socket.socket(family=socket.AF_INET, type=socket.SOCK_STREAM)
s.bind(("localhost", 0))
addr = s.getsockname()
s.connect(addr)

pid = os.fork()
if pid == 0:
    # child
    s.send(b'foo')
    s.close()
else:
    # parent: recv, then wait for the child to exit, and only then print the
    # port, so the Go test polls after both sides have updated their stats.
    # recv alone is not enough: on loopback the data reaches the parent while
    # the child is still inside tcp_sendmsg, so the child's sent bytes can be
    # recorded after the parent's recv returns.
    s.recv(256)
    os.wait()
    print(addr[1], flush=True)
    signal.pause()
