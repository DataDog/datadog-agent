#!/usr/bin/env bash

set -e

VENV_PATH=./p-env

if [[ ! -d $VENV_PATH ]]; then
  virtualenv  $VENV_PATH
  source $VENV_PATH/bin/activate
  pip install -r molecule-role/requirements.txt
else
  source $VENV_PATH/bin/activate
fi

cd molecule-role

molecule "$@"
