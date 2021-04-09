# StackState Agent

[![CircleCI](https://circleci.com/gh/StackVista/stackstate-agent/tree/master.svg?style=svg)](https://circleci.com/gh/StackVista/stackstate-agent/tree/master)
[![Build status](https://ci.appveyor.com/api/projects/status/kcwhmlsc0oq3m49p/branch/master?svg=true)](https://ci.appveyor.com/project/StackVista/stackstate-agent/branch/master)
[![GoDoc](https://godoc.org/github.com/StackVista/stackstate-agent?status.svg)](https://godoc.org/github.com/StackVista/stackstate-agent)
[![Go Report Card](https://goreportcard.com/badge/github.com/StackVista/stackstate-agent)](https://goreportcard.com/report/github.com/StackVista/stackstate-agent)

The present repository contains the source code of the Datadog Agent version 6. Please refer to the [Agent user documentation](docs/agent) for information about differences between Agent 5 and Agent 6. Additionally, we provide a list of prepackaged binaries for an easy install process [here](https://app.datadoghq.com/account/settings#agent)
**Note:** the source code of Datadog Agent 5 is located in the

## Documentation

The general documentation of the project, including instructions for installation
and development, is located under [the docs directory](docs) of the present repo.

## Getting started

To build the Agent you need:
 * [Go](https://golang.org/doc/install) 1.13 or later. You'll also need to set your `$GOPATH` and have `$GOPATH/bin` in your path.
 * Python 2.7 or 3.7+ along with development libraries.
 * Python dependencies. You may install these with `pip install -r requirements.txt`
   This will also pull in [Invoke](http://www.pyinvoke.org) if not yet installed.
 * CMake version 3.12 or later and a C++ compiler

**Note:** you may want to use a python virtual environment to avoid polluting your
      system-wide python environment with the agent build/dev dependencies. You can
      create a virtual environment using `virtualenv` and then use the `invoke` parameter
      `--python-home-2=<venv_path>` and/or `--python-home-3=<venv_path>` (depending on
      the python versions you are using) to use the virtual environment's interpreter
      and libraries. By default, this environment is only used for dev dependencies
      listed in `requirements.txt`.

**Note:** You may have previously installed `invoke` via brew on MacOS, or `pip` in
      any other platform. We recommend you use the version pinned in the requirements
      file for a smooth development/build experience.

Builds and tests are orchestrated with `invoke`, type `invoke --list` on a shell
to see the available tasks.

To start working on the Agent, you can build the `master` branch:

1. Checkout the repo: `git clone https://github.com/StackVista/stackstate-agent.git $GOPATH/src/github.com/StackVista/stackstate-agent`.
2. cd into the project folder: `cd $GOPATH/src/github.com/StackVista/stackstate-agent`.
3. Install project's dependencies: `invoke deps`.
   Make sure that `$GOPATH/bin` is in your `$PATH` otherwise this step might fail.
4. Create a development `datadog.yaml` configuration file in `dev/dist/datadog.yaml`, containing a valid API key: `api_key: <API_KEY>`
5. Build the agent with `invoke agent.build --build-exclude=systemd`.
   By default, the Agent will be built to use Python 3 but you can select which Python version you want to use:
   - `invoke agent.build --python-runtimes 2` for Python2 only
   - `invoke agent.build --python-runtimes 3` for Python3 only
   - `invoke agent.build --python-runtimes 2,3` for both Python2 and Python3
  You can specify a custom Python location for the agent (useful when using
   virtualenvs): `invoke agent.build
   --python-runtimes 2,3
   --python-home-2=$GOPATH/src/github.com/StackVista/stackstate-agent/venv2
   --python-home-3=$GOPATH/src/github.com/StackVista/stackstate-agent/venv3`.
  Running `invoke agent.build`:
    * Discards any changes done in `bin/agent/dist`.
    * Builds the Agent and writes the binary to `bin/agent/agent`.
    * Copies files from `dev/dist` to `bin/agent/dist`. See `https://github.com/StackVista/stackstate-agent/blob/master/dev/dist/README.md` for more information.
  If you built an older version of the agent, you may have the error `make: *** No targets specified and no makefile found.  Stop.`.
  To solve the issue, you should remove `CMakeCache.txt` from `rtloader` folder with `rm rtloader/CMakeCache.txt`.



Please refer to the [Agent Developer Guide](docs/dev/README.md) for more details.

## Run

You can run the agent with:
```
./bin/agent/agent run -c bin/agent/dist/stackstate.yaml
```

The file `bin/agent/dist/datadog.yaml` is copied from `dev/dist/datadog.yaml` by `invoke agent.build` and must contain a valid api key.

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
