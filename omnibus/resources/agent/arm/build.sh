#!/bin/bash
# http://redsymbol.net/articles/unofficial-bash-strict-mode/
IFS=$'\n\t'
set -euxo pipefail

# version
AGENT_VERSION=master  # FIXME: Update the version to a stable tag

# build dependencies
export DEBIAN_FRONTEND=noninteractive
apt-get update
apt-get install -y python-dev python-virtualenv git curl mercurial bundler

# The agent loads systemd at runtime https://github.com/coreos/go-systemd/blob/a4887aeaa186e68961d2d6af7d5fbac6bd6fa79b/sdjournal/functions.go#L46
# which means it doesn't need to be included in the omnibus build
# but it requires the headers at build time https://github.com/coreos/go-systemd/blob/a4887aeaa186e68961d2d6af7d5fbac6bd6fa79b/sdjournal/journal.go#L27
apt-get install -y libsystemd-dev

export PATH=$GOPATH/bin:$PATH

mkdir -p $GOPATH/src/github.com/DataDog

# git needs this to apply the patches with `git am` we don't actually care about the committer name here
git config --global user.email "you@example.com"
git config --global user.name "Your Name"

##########################################
#               MAIN AGENT               #
##########################################

# clone the main agent
git clone https://github.com/DataDog/datadog-agent $GOPATH/src/github.com/DataDog/datadog-agent

(
  cd $GOPATH/src/github.com/DataDog/datadog-agent
  git checkout $AGENT_VERSION
  git tag "$AGENT_VERSION-armv7"

  # create virtualenv to hold pip deps
  virtualenv $GOPATH/venv
  set +u; source $GOPATH/venv/bin/activate; set -u

  # install build dependencies
  pip install -r requirements.txt

  # build the agent
  invoke -e agent.omnibus-build --base-dir=$HOME/.omnibus --release-version=$AGENT_VERSION
)

set +u
# build the image
if [ ! -z $DOCKER_USERNAME ]; then
    cd $GOPATH/src/github.com/DataDog/datadog-agent/Dockerfiles/agent
    cp "$HOME/.omnibus/pkg/*.deb" .
    docker login -u="$DOCKER_USERNAME"
    docker build . -t $DOCKER_USERNAME/datadog-agent:$AGENT_VERSION
    docker push "$DOCKER_USERNAME/datadog-agent:$AGENT_VERSION-armv7"
fi
