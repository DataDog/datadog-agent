# Datadog Agent

[![CircleCI](https://circleci.com/gh/DataDog/datadog-agent/tree/master.svg?style=svg&circle-token=dbcee3f02b9c3fe5f142bfc5ecb735fdec34b643)](https://circleci.com/gh/DataDog/datadog-agent/tree/master)

The Datadog Agent faithfully collects events and metrics and brings them to
[Datadog](https://app.datadoghq.com) on your behalf so that you can do something
useful with your monitoring and performance data.

The present repository contains the source code of the Datadog Agent version 6,
currently in Beta. The source code of the stable Datadog Agent 5 is located in the
[dd-agent](https://github.com/DataDog/dd-agent) repository.

## Getting started

To build the Agent you need:
 * [Go](https://golang.org/doc/install) 1.8 or later.
 * Python 2.7 along with development libraries.
 * [Invoke](http://www.pyinvoke.org/installing.html), you can install it via
   `pip install invoke` or via [Homebrew](https://brew.sh) on OSX/macOS with
   `brew install invoke`.

Builds and tests are orchestrated with `invoke`, type `invoke --list` on a shell
to see the available tasks.

To start working on the Agent, you can build the `master` branch:

1. checkout the repo within your `$GOPATH`.
2. install the project's dependencies: `invoke deps`.
   Make sure that `$GOPATH/bin` is in your `$PATH` otherwise this step might fail.
3. build the whole project with `invoke agent.build --build-exclude=snmp`

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

The general documentation of the project (including instructions on the Beta builds, Agent installation,
development, etc) is located under the [docs](docs) directory of the present repo.

## Contributing code

You must sign a CLA before we can accept your contributions. If you submit a PR
without having signed it, our bot will prompt you to do so. Once signed you will
be set for all contributions going forward.
