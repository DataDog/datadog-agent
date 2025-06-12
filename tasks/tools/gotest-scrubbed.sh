#!/bin/bash
### This script is used to run go test and scrub the output, the command can be used as follow:
### ./gotest-scrubbed.sh <packages comma separated> -- <go tests flags>
set -euo pipefail
./gotest-custom "$1" "${@:3}" | 
gsed -E 's/\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b/**************************\1/g' | # Scrub API keys
gsed -E 's/\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b/************************************\1/g' # Scrub APP keys
