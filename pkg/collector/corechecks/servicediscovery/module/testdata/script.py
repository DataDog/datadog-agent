#! /usr/bin/python3

import socket
import time

HOST = '127.0.0.1'
PORT = 0  # Empty port

s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
s.bind((HOST, PORT))
s.listen()
time.sleep(30)
