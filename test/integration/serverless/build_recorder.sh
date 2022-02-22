#!/bin/bash

echo "Building recorder extension"
cd recorder-extension
GOOS=linux GOARCH=$ARCHITECTURE go build -o extensions/recorder-extension-$ARCHITECTURE main.go
zip -rq ext-$ARCHITECTURE.zip extensions/recorder-extension-$ARCHITECTURE