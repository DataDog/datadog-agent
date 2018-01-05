# Datadog Agent

[![CircleCI](https://circleci.com/gh/DataDog/datadog-agent/tree/master.svg?style=svg&circle-token=dbcee3f02b9c3fe5f142bfc5ecb735fdec34b643)](https://circleci.com/gh/DataDog/datadog-agent/tree/master)
[![Build status](https://ci.appveyor.com/api/projects/status/kcwhmlsc0oq3m49p/branch/master?svg=true)](https://ci.appveyor.com/project/Datadog/datadog-agent/branch/master)
[![GoDoc](https://godoc.org/github.com/DataDog/datadog-agent?status.svg)](https://godoc.org/github.com/DataDog/datadog-agent)

The present repository contains the source code of the Datadog Agent version 6,
**currently in beta**. Please refer to the [beta docs](docs/beta.md) for more
informations about the status of the project, the limitations and how to install
the latest version of the Agent.

**Note:** the source code of the **stable** Datadog Agent 5 is located in the
[dd-agent](https://github.com/DataDog/dd-agent) repository.

## Getting started

To build the Agent you need:
 * [Go](https://golang.org/doc/install) 1.9.2 or later.
 * Python 2.7 along with development libraries.
 * [Invoke](http://www.pyinvoke.org/installing.html), you can install it via
   `pip install invoke` or via [Homebrew](https://brew.sh) on OSX/macOS with
   `brew install pyinvoke`.

Builds and tests are orchestrated with `invoke`, type `invoke --list` on a shell
to see the available tasks.

To start working on the Agent, you can build the `master` branch:

1. checkout the repo: `git clone https://github.com/DataDog/datadog-agent.git $GOPATH/src/github.com/DataDog/datadog-agent`.
2. cd into the project folder: `cd $GOPATH/src/github.com/DataDog/datadog-agent`.
3. install project's dependencies: `invoke deps`.
   Make sure that `$GOPATH/bin` is in your `$PATH` otherwise this step might fail.
4. build the whole project with `invoke agent.build --build-exclude=snmp`

Please refer to the [Agent Developer Guide](docs/dev/README.md) for more details.

## Run

To start the agent type `agent start` from the `bin/agent` folder, it will take
care of adjusting paths and run the binary in foreground.

You need to provide a valid API key, either through the config file or passing
the environment variable like:
```
DD_API_KEY=12345678990 ./bin/agent/agent
```

## Documentation

The general documentation of the project (including instructions on the Beta builds,
Agent installation, development, etc) is located under the [docs](docs) directory
of the present repo.

## Contributing code

You must sign a CLA before we can accept your contributions. If you submit a PR
without having signed it, our bot will prompt you to do so. Once signed you will
be set for all contributions going forward.
