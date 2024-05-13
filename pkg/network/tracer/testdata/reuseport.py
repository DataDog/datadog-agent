#!/usr/bin/env python3

import multiprocessing
import os
import random
import socket
import sys

# for synchronizing children and parent
barrier = multiprocessing.Barrier(3)
children = []
port = random.randrange(32768, 65535)
print(port)
count = range(2)
for _ in count:
    child = os.fork()
    if child:
        children.append(child)
        continue

    # child
    s = socket.socket(family=socket.AF_INET, type=socket.SOCK_DGRAM, proto=socket.IPPROTO_UDP)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEPORT, 1)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    s.bind(("localhost", port))
    s.settimeout(3)
    barrier.wait()
    _, addr = s.recvfrom(1024)
    print("child: received from " + str(addr))
    s.sendto(b'bar', addr)
    print("child: sent to " + str(addr))

    s.close()
    sys.exit()

barrier.wait()
conns = []
print(children)
for _ in count:
    c = socket.socket(family=socket.AF_INET, type=socket.SOCK_DGRAM, proto=socket.IPPROTO_UDP)
    c.settimeout(3)
    c.sendto(b'foobar', ("localhost", port))
    print("parent: sent")
    _, addr = c.recvfrom(1024)
    print("parent: received from " + str(addr))
    conns.append(c)

for c in conns:
    c.close()

for child in children:
    _, rc = os.waitpid(child, 0)
    assert rc == 0, "child process exited with non-zero exit code"
