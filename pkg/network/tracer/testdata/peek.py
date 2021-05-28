#!/usr/bin/env python3

import socket

s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.bind(("127.0.0.1", 34568))
s.recvfrom(1024, socket.MSG_PEEK)
s.recvfrom(1024)
s.close()
