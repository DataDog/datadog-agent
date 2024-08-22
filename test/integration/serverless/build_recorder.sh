#!/bin/bash

echo "Building recorder extension"
cd recorder-extension
GOOS=linux CGO_ENABLED=1 CC=gcc GOARCH=$ARCHITECTURE go build -o extensions/recorder-extension main.go
zip -rq ext.zip extensions/recorder-extension
