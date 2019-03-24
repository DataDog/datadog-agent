#!/usr/bin/env bash

# This file is meant to be sourced
# This file will pull in deps when a test is ran without the deps job having ran first.
# This happens when using the ./run_gitlab_local.sh script

set -x

VENV_PATH=$CI_PROJECT_DIR/venv

cd /go/src/github.com/StackVista/stackstate-agent

if [ ! -d $VENV_PATH ]; then
  virtualenv --python=python2.7 $CI_PROJECT_DIR/venv
  source $CI_PROJECT_DIR/venv/bin/activate
  pip install -r requirements.txt
else
  source $CI_PROJECT_DIR/venv/bin/activate
fi

if [ ! -d $CI_PROJECT_DIR/vendor ]; then
  inv deps
fi

set +x
