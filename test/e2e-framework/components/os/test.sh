#!/bin/bash

export PACKAGES=("datadog-agent" "apm-lib2" "test")

for pkg in "${PACKAGES[@]}"; do
    echo "Installing $pkg"
    echo "Done"
done
