# Build and distribute Stackstate Agent in linux using Docker

Using our builder image clone and checkout the public repo: 
```bash
$ docker run --rm -ti docker.io/stackstate/stackstate-agent-runner-gitlab:latest7 bash

$ export CI_PROJECT_DIR=/go/src/github.com/StackVista/stackstate-agent && \
  mkdir -p /go/src/github.com/StackVista && \
  cd src/github.com/StackVista && \
  git clone https://github.com/StackVista/stackstate-agent && \
  cd stackstate-agent && \
  git checkout upstream-updates-7-21
```

Remember to `git pull` every time you push a change.

### Configure Artifactory

We use some private python libraries for our integrations therefore you need to configure artifactory as pypi repository:
```bash
$ export ARTIFACTORY_URL=artifactory.stackstate.io/artifactory/api/pypi/pypi-local && \
  export ARTIFACTORY_USER=... && \
  export ARTIFACTORY_PASSWORD=... && \
  source ./.gitlab-scripts/setup_artifactory.sh
```

### Build using Python3 interpreter
```bash
$ conda activate ddpy3 && \
  inv deps && \
  inv agent.clean && \
  inv -e agent.omnibus-build --base-dir /omnibus --skip-deps --skip-sign --major-version 2 --python-runtimes 3
```

### Build using Python2 interpreter
```bash
$ conda activate ddpy2 && \
  inv deps && \
  inv agent.clean && \
  inv -e agent.omnibus-build --base-dir /omnibus --skip-deps --skip-sign --major-version 2 --python-runtimes 2
```

## Use local copy

Instead of cloning the repo you could use directly your local one:
```bash
$ docker run --rm -it --name stackstate-agent-builder --mount type=bind,source="${PWD}",target=/root/stackstate-agent,readonly docker.io/stackstate/stackstate-agent-runner-gitlab:latest7 bash

$ export CI_PROJECT_DIR=/go/src/github.com/StackVista/stackstate-agent && \
  mkdir -p /go/src/github.com/StackVista && \
  cd src/github.com/StackVista
  
$ cp -r /root/stackstate-agent /go/src/github.com/StackVista
```

Now [configure Artifatory](#configure-artifactory) then build using either [Python2](#build-using-python2-interpreter) or [Python3](#build-using-python3-interpreter).
Remember to copy every time you make a change on your local copy.
