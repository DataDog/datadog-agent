#!/usr/bin/env bash

# This file is meant to be sourced
# This file will pull in deps when a test is ran without the deps job having ran first.
# This happens when using the ./run_gitlab_local.sh script

set -x

SETUP_DIR=${CI_PROJECT_DIR:-"."}
VENV_PATH=$SETUP_DIR/venv

cd /go/src/github.com/StackVista/stackstate-agent

if [ ! -d $VENV_PATH ]; then
  virtualenv --python=python2.7 $SETUP_DIR/venv
  source $SETUP_DIR/venv/bin/activate
  pip install -r requirements.txt
else
  source $SETUP_DIR/venv/bin/activate
fi

if [ ! -d $SETUP_DIR/vendor ]; then
  inv deps
fi

set +x
