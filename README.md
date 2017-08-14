# Datadog Agent

[![CircleCI](https://circleci.com/gh/DataDog/datadog-agent/tree/master.svg?style=svg&circle-token=dbcee3f02b9c3fe5f142bfc5ecb735fdec34b643)](https://circleci.com/gh/DataDog/datadog-agent/tree/master)

This repo contains the code needed to build different tools: for more informations about specific projects,
see the README files for:
 * [Agent](cmd/agent/README.md)
 * [Dogstatsd](cmd/dogstatsd/README.md)

## Requirements
To build the Agent you need:
 * `go` 1.8+.
 * A Python 2.7.x environment along with development libraries.
 * [Invoke](http://www.pyinvoke.org/installing.html), you can install it via
   `pip install invoke` or via [Homebrew](https://brew.sh) on Mac OSX with
   `brew install invoke`.

Builds and tests are orchestrated with `invoke`, type `invoke --list` on a shell
to see the available tasks.

If you want to build the system packages, you also need a recent and working
`ruby` environment with `bundler` installed.

We use `pkg-config` to make compilers and linkers aware of Python. If you need
to adjust the build for your specific configuration, add or edit the files within
the `pkg-config` folder.

## Getting started
To start working on the Agent, you can build the `master` branch:

1. checkout the repo within your `GOPATH`.
2. install the project's dependencies: `invoke deps`.
   Make sure that `GOPATH/bin` is in your `PATH` otherwise this step might fail.
   Alternatively  you can install [dep](https://github.com/golang/dep) manually
   on your system before running `invoke deps`.
3. build the whole project with `invoke build`, see [the Agent README](cmd/agent/README.md)
   for more details on how to build the Agent alone.

## Tests
Some tests have specific requirements, see [System Tests](test/README.md).

## Contributing

In order for your contributions you will be required to sign a CLA. When a PR is opened a bot will prompt you to sign the CLA. Once signed you will be set for all contributions going forward.
