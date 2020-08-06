#!/bin/bash -l
set -e

source /root/.bashrc

if command -v conda; then
  # Only try to use conda if it's installed.
  # On ARM images, we use the system Python 3 because conda is not supported.
  conda activate ddpy3
fi

#if [ "$DD_TARGET_ARCH" = "arm64v8" ] ; then
#    export GIMME_ARCH=arm64
#elif [ "$DD_TARGET_ARCH" = "arm32v7" ] ; then
#    export GIMME_ARCH=arm
#fi
#eval "$(gimme)"

exec "$@"
