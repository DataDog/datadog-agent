#!/bin/sh
set -e
cd "$(dirname "$0")"

speky ddot.yaml -o markdown
sphinx-build -M html markdown sphinx --conf-dir .
