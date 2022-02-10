#!/bin/bash

echo "Building recorder extension"
cd recorder-extension
GOOS=linux GOARCH=amd64 go build -o extensions/recorder-extension main.go
zip -rq ext.zip extensions/* -x ".*" -x "__MACOSX" -x "extensions/.*"