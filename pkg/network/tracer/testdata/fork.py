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
    # parent: recv first, then print the port so the Go test only reads it
    # after both tcp_sendmsg (child) and tcp_recvmsg (parent) BPF probes
    # have fired. This avoids a race where the first GetActiveConnections
    # poll happens before the parent's recv updates the BPF map entry.
    s.recv(256)
    print(addr[1], flush=True)
    os.wait()
    signal.pause()
