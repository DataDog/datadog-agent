#!/usr/bin/env bash

set -e

VENV_PATH=./p-env

if [[ -z $CI_COMMIT_REF_NAME ]]; then
  export AGENT_CURRENT_BRANCH=`git rev-parse --abbrev-ref HEAD`
else
  export AGENT_CURRENT_BRANCH=$CI_COMMIT_REF_NAME
fi

if [[ ! -d $VENV_PATH ]]; then
  virtualenv --python=python2 $VENV_PATH
  source $VENV_PATH/bin/activate
  pip install -r molecule-role/requirements.txt
else
  source $VENV_PATH/bin/activate
fi

cd molecule-role

#echo =====MOLECULE_RUN_ID=${CI_JOB_ID}======AGENT_CURRENT_BRANCH=${CI_COMMIT_REF_NAME}=======

molecule "$@"
