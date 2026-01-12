#!/bin/sh
set -e
cd "$(dirname "$0")"

cc -g parser-test.c -o parser-test

./parser-test