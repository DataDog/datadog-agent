#!/bin/env bash

PYTHON_EXE="$1"
MODULE="$2"

$PYTHON_EXE -c "import $MODULE"
