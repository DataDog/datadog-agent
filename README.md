# Datadog Agent

[![CircleCI](https://circleci.com/gh/DataDog/datadog-agent/tree/master.svg?style=svg&circle-token=dbcee3f02b9c3fe5f142bfc5ecb735fdec34b643)](https://circleci.com/gh/DataDog/datadog-agent/tree/master)

This repo contains the code needed to build different tools: for more informations about specific projects,
see the README files for:
 * [Agent](cmd/agent/README.md)
 * [Dogstatsd](cmd/dogstatsd/README.md)

Builds and tests are orchestrated by a `Rakefile`, issue `rake -T` on a shell to see the available tasks.

## Requirements
To build the Agent you need:
 * `go` 1.8+.
 * `rake`.
 * a working Python 2.7.x environment along with development libraries; alternatively, an Agent already installed through the system packages Datadog provides.

We use `pkg-config` to make compilers and linkers aware of Python. If you need to adjust the build for your specific configuration, add or edit the files within the `pkg-config` folder.

## Getting started
To start working on the Agent, you can build the `master` branch:

1. checkout the repo within your `GOPATH`.
2. install the project's dependencies: `rake deps`.
   Make sure that `GOPATH/bin` is in your `PATH` otherwise this step might fail. Alternatively  you can
   install [glide](https://github.com/Masterminds/glide) manually on your system before running `rake deps`.
3. build the whole project with `rake build`, see [the Agent README](cmd/agent/README.md) for more details
   on how to build the Agent alone.

## Tests
Some tests have specific requirements, see [System Tests](test/README.md).
