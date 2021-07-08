#!/usr/bin/env bash
export CONDA_BASE="${HOME}/miniconda3"

# see if conda is available -- when running locally and use the conda base path
if [ -x "$(command -v conda)" ]; then
    CONDA_BASE=$(conda info --base)
fi

source $CONDA_BASE/etc/profile.d/conda.sh
conda env list | grep 'molecule' &> /dev/null
if [ $? != 0 ]; then
   conda create -n molecule python=3.6.12 -y || true
fi

set -e

export STACKSTATE_BRANCH=${STACKSTATE_BRANCH:-master}

export MAJOR_VERSION=${MAJOR_VERSION:-3}
export STS_AWS_TEST_BUCKET=${STS_AWS_TEST_BUCKET:-stackstate-agent-3-test}
export STS_DOCKER_TEST_REPO=${STS_DOCKER_TEST_REPO:-stackstate-agent-test}
export STS_DOCKER_TEST_REPO_CLUSTER=${STS_DOCKER_TEST_REPO_CLUSTER:-stackstate-cluster-agent-test}

if [[ -z $CI_COMMIT_REF_NAME ]]; then
  export AGENT_CURRENT_BRANCH=`git rev-parse --abbrev-ref HEAD`
else
  export AGENT_CURRENT_BRANCH=$CI_COMMIT_REF_NAME
fi

conda activate molecule

pip3 install -r molecule-role/requirements-molecule3.txt

# reads env file to file variables for molecule jobs locally
ENV_FILE=./.env
if test -f "$ENV_FILE"; then
    echo "===== Sourcing env file with contents ======="
    echo "$(cat $ENV_FILE)"
    echo "============================================="
    source $ENV_FILE
fi

cd molecule-role

echo "===== MOLECULE_RUN_ID=${CI_JOB_ID:-unknown}  ======="
echo "====== AGENT_CURRENT_BRANCH=${AGENT_CURRENT_BRANCH} ======="

molecule "$@"
