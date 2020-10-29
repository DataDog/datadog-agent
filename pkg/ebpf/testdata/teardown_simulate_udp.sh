#!/usr/bin/env bash

set -x

LOCALHOST=127.0.0.1
PORT=8081

pkill -f "nc -u -l $LOCALHOST $PORT"
pkill -f "nc -u $LOCALHOST $PORT"
