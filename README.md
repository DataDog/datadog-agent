# Datadog Agent

[![CircleCI](https://circleci.com/gh/DataDog/datadog-agent/tree/master.svg?style=svg&circle-token=dbcee3f02b9c3fe5f142bfc5ecb735fdec34b643)](https://circleci.com/gh/DataDog/datadog-agent/tree/master)
[![Build status](https://ci.appveyor.com/api/projects/status/kcwhmlsc0oq3m49p/branch/master?svg=true)](https://ci.appveyor.com/project/Datadog/datadog-agent/branch/master)
[![Coverage status](https://codecov.io/github/DataDog/datadog-agent/coverage.svg?branch=master)](https://codecov.io/github/DataDog/datadog-agent?branch=master)
[![GoDoc](https://godoc.org/github.com/DataDog/datadog-agent?status.svg)](https://godoc.org/github.com/DataDog/datadog-agent)
[![Go Report Card](https://goreportcard.com/badge/github.com/DataDog/datadog-agent)](https://goreportcard.com/report/github.com/DataDog/datadog-agent)

The present repository contains the source code of the Datadog Agent version 7 and version 6. Please refer to the [Agent user documentation](docs/agent) for information about differences between Agent v5, Agent v6 and Agent v7. Additionally, we provide a list of prepackaged binaries for an easy install process [here](https://app.datadoghq.com/account/settings#agent)

**Note:** the source code of Datadog Agent v5 is located in the
[dd-agent](https://github.com/DataDog/dd-agent) repository.

## Documentation

The general documentation of the project, including instructions for installation
and development, is located under [the docs directory](docs) of the present repo.

## Getting started

To build the Agent you need:
 * [Go](https://golang.org/doc/install) 1.11.5 or later. You'll also need to set your `$GOPATH` and have `$GOPATH/bin` in your path.
 * Python 2.7 or 3.x along with development libraries.
 * Python dependencies. You may install these with `pip install -r requirements.txt`
   This will also pull in [Invoke](http://www.pyinvoke.org) if not yet installed.
 * CMake version 3.12 or later

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

1. Checkout the repo: `git clone https://github.com/DataDog/datadog-agent.git $GOPATH/src/github.com/DataDog/datadog-agent`.
2. cd into the project folder: `cd $GOPATH/src/github.com/DataDog/datadog-agent`.
3. Install project's dependencies: `invoke deps`.
   Make sure that `$GOPATH/bin` is in your `$PATH` otherwise this step might fail.
4. Build the `rtloader` dependency with `invoke rtloader.build && invoke
   rtloader.install`. You will need CMake installed and a C++ compiler for this
   to work.
   `rtloader` is in charge of loading and running Python. By default `rtloader`
   will be built for Python2, but you can choose which versions of Python you want
   to support:
   - `invoke rtloader.build -p 2` for Python2 only
   - `invoke rtloader.build -p 3` for Python3 only
   - `invoke rtloader.build -p 2,3` for both Python2 and Python3
5. Build the agent with `invoke agent.build --build-exclude=systemd`. You can
   specify a custom Python location for the agent (useful when using
   virtualenvs): `invoke agent.build
   --python-home-2=$GOPATH/src/github.com/DataDog/datadog-agent/venv2
   --python-home-3=$GOPATH/src/github.com/DataDog/datadog-agent/venv3`.

Please refer to the [Agent Developer Guide](docs/dev/README.md) for more details.

## Run

To start the agent type `agent run` from the `bin/agent` folder, it will take
care of adjusting paths and run the binary in foreground.

You need to provide a valid API key. You can either use the config file or
overwrite it with the environment variable like:
```
DD_API_KEY=12345678990 ./bin/agent/agent run -c bin/agent/dist/datadog.yaml
```

## Contributing code

You'll find information and help on how to contribute code to this project under
[the `docs/dev` directory](docs/dev) of the present repo.
