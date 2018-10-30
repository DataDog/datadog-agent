# This file is meant to be sourced
# This file will pull in deps when a test is ran without the deps job having ran first.
# This happens when using the ./run_gitlab_local.sh script
set -x

virtualenv  $CI_PROJECT_DIR/venv
source $CI_PROJECT_DIR/venv/bin/activate
cd /go/src/github.com/StackVista/stackstate-agent
pip install -r requirements.txt
inv deps

set +x
