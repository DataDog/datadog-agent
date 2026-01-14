#!/usr/bin/env bash

tool="$PWD/{{tool}}"
cd "${{env_var}}" && exec "$tool" "$@"
