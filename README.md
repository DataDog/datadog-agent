# StackState Agent

[![CircleCI](https://circleci.com/gh/StackVista/stackstate-agent/tree/master.svg?style=svg)](https://circleci.com/gh/StackVista/stackstate-agent/tree/master)
[![Build status](https://ci.appveyor.com/api/projects/status/kcwhmlsc0oq3m49p/branch/master?svg=true)](https://ci.appveyor.com/project/StackVista/stackstate-agent/branch/master)
[![GoDoc](https://godoc.org/github.com/StackVista/stackstate-agent?status.svg)](https://godoc.org/github.com/StackVista/stackstate-agent)
[![Go Report Card](https://goreportcard.com/badge/github.com/StackVista/stackstate-agent)](https://goreportcard.com/report/github.com/StackVista/stackstate-agent)

The present repository contains the source code of the StackState Agent version 6.

## Documentation

The general documentation of the project, including instructions for installation
and development, is located under [the docs directory](docs) of the present repo.

## Getting started

To build the Agent you need:
 * [Go](https://golang.org/doc/install) 1.11.5 or later.
 * Python 2.7 along with development libraries.
 * Python dependencies. You may install these with `pip install -r requirements.txt`
   This will also pull in [Invoke](http://www.pyinvoke.org) if not yet installed.

**Note:** you may want to use a python virtual environment to avoid polluting your
      system-wide python environment with the agent build/dev dependencies. By default, this environment is only used for dev dependencies listed in `requirements.txt`, if you want the agent to use the virtual environment's interpreter and libraries instead of the system python's ones,
      add `--use-venv` to the build command.

**Note:** You may have previously installed `invoke` via brew on MacOS, or `pip` in
      any other platform. We recommend you use the version pinned in the requirements
      file for a smooth development/build experience.

Builds and tests are orchestrated with `invoke`, type `invoke --list` on a shell
to see the available tasks.

To start working on the Agent, you can build the `master` branch:

1. checkout the repo: `git clone https://github.com/StackVista/stackstate-agent.git $GOPATH/src/github.com/StackVista/stackstate-agent`.
2. cd into the project folder: `cd $GOPATH/src/github.com/StackVista/stackstate-agent`.
3. install project's dependencies: `invoke deps`.
   Make sure that `$GOPATH/bin` is in your `$PATH` otherwise this step might fail.
4. build the whole project with `invoke agent.build --build-exclude=snmp,systemd` (with `--use-venv` to use a python virtualenv)

Please refer to the [Agent Developer Guide](docs/dev/README.md) for more details.

## Run

To start the agent type `agent run` from the `bin/agent` folder, it will take
care of adjusting paths and run the binary in foreground.

You need to provide a valid API key. You can either use the config file or
overwrite it with the environment variable like:
```
STS_API_KEY=12345678990 ./bin/agent/agent -c bin/agent/dist/stackstate.yaml
```

## Contributing code

You'll find information and help on how to contribute code to this project under
[the `docs/dev` directory](docs/dev) of the present repo.

## Install

### Linux

##### Official

To install the official release:

    $ curl -o- https://stackstate-agent-2.s3.amazonaws.com/install.sh | STS_API_KEY="xxx" STS_URL="yyy" bash
     or
    $ wget -qO- https://stackstate-agent-2.s3.amazonaws.com/install.sh | STS_API_KEY="xxx" STS_URL="yyy" bash

##### Test

If you wanna install a branch version use the test repository:

    $ curl -o- https://stackstate-agent-2-test.s3.amazonaws.com/install.sh | STS_API_KEY="xxx" STS_URL="yyy" CODE_NAME="PR_NAME" bash
     or
    $ wget -qO- https://stackstate-agent-2-test.s3.amazonaws.com/install.sh | STS_API_KEY="xxx" STS_URL="yyy" CODE_NAME="PR_NAME" bash

and replace `PR_NAME` with the branch name (e.g. `master`, `STAC-xxxx`).

### Docker

##### Official

    $ docker pull docker.io/stackstate/stackstate-agent-2:latest

##### Test

    $ docker pull docker.io/stackstate/stackstate-agent-2-test:latest

### Windows

##### Official

To install the official release:

    $ . { iwr -useb https://stackstate-agent-2.s3.amazonaws.com/install.ps1 } | iex; install -stsApiKey "xxx" -stsUrl "yyy"

##### Test

If you wanna install a branch version use the test repository:

    $ . { iwr -useb https://stackstate-agent-2-test.s3.amazonaws.com/install.ps1 } | iex; install -stsApiKey "xxx" -stsUrl "yyy" -codeName "PR_NAME"

and replace `PR_NAME` with the branch name (e.g. `master`, `STAC-xxxx`).

#### Arguments

Other arguments can be passed to the installation command.

Linux arguments:

- `STS_HOSTNAME` = Instance hostname
- `$HOST_TAGS` = Agent host tags to use for all topology component (by default `os:linux` will be added)
- `SKIP_SSL_VALIDATION` = Skip ssl certificates validation when talking to the backend (defaults to `false`)
- `STS_INSTALL_ONLY` = Agent won't be automatically started after installation

Windows arguments:

- `hostname` = Instance hostname
- `tags` = Agent host tags to use for all topology component (by default `os:windows` will be added)
- `skipSSLValidation` = Skip ssl certificates validation when talking to the backend (defaults to `false`)
- `agentVersion` = Version of the Agent to be installed (defaults to `latest`)
