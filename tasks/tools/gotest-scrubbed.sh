#!/bin/bash
### This script is used to run go test and scrub the output of API keys, the input should be of the form:
### ./gotest-scrubbed.sh <packages comma separated> -- <go tests flags>
go test -json "$1" "${@:3}" | 
sed -E 's/\b[a-fA-F0-9]{27}([a-fA-F0-9]{5})\b/**************************\1/g' | # Scrub API keys
sed -E 's/\b[a-fA-F0-9]{35}([a-fA-F0-9]{5})\b/************************************\1/g' # Scrub APP keys
