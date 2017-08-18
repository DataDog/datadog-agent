# Setting up your development environment

## Invoke

[Invoke](http://www.pyinvoke.org/installing.html) is a task runner written in
Python that is extensively used to orchestrate builds and test runs.

The easiest way to install it on any supported platform is using `pip`:
```
pip install invoke
```

OSX users can install it via [Homebrew](https://brew.sh) with:
```
brew install invoke
```

Tasks are usually parametrised and Invoke comes with some default values that
are used in the official build. Such values are listed in the `invoke.yaml`
file at the root of this repo and can be overridden by setting `INVOKE_*` env
variables (see Invoke docs for more details).

## Golang

You must install [go](https://golang.org/doc/install) version 1.8 or above. Make
sure that `GOPATH/bin` is in your `PATH` (all platforms) otherwise Invoke cannot
use any additional tool it might need.

## System or Embedded?

When building system packages with Omnibus, all the external dependencies needed
are built locally and put in the Omnibus cache. Such dependencies are then used
to build the Agent binary that will be included in the final package.

Despite this is not expected to be that common, it might be desirable to build
the agent using the very same bits that are used in the official packages even
in a development enviroment. This behavior can be configured adjusting the
`use_system_libs` boolean flag (either setting the proper env var, changing the
`invoke.yaml` file or passing the corresponding arg to `invoke`). If you set
`use_sytem_libs: false` and run Omnibus, you don't need any external dependency
to build the Agent, though you might need to setup your dev env to build such
dependencies, so don't think this is a shortcut.

If you don't care about building an exact clone of the official Agent at the
binary level, and this should be the case most of the times, you can set
`use_sytem_libs: true` and avoid running Omnibus, which might be quite time
consuming. In this case you need to provide the external dependencies by
yourself, go ahead to see how to do it.

### Python

The Agent embeds a full fledget CPython interpreter so it requires the development
files to be available in the dev env.

If you're on OSX, installing Python 2.7 with [Homebrew](https://brew.sh) will
bring along all the development files needed:
```
brew install python
```

On Windows, the [official installer](https://www.python.org/downloads/) will
provide all the files needed.

On Linux, depending on the distribution, you might need to explicitly install
the development files, for example on Ubuntu:
```
sudo apt-get install python2.7-dev
```

### SNMP (Simple Network Management Protocol)

The new SNMP check is written in Go, so the Agent must be built against few
libraries.

On OSX with [Homebrew](https://brew.sh):
```
brew install net-snmp
```

On Windows TODO

On Ubuntu:
```
sudo apt-get install libsnmp-base libsnmp-dev snmp-mibs-downloader
```

## Building the system packages

The Agent uses [Omnibus](https://github.com/chef/omnibus) to build the official
packages for all the platforms Datadog supports. In order to build a system
package, and only in this case, you need a recent and working `ruby` environment
with `bundler` installed.

## Docker

If you want to build a Docker image containing the Agent, or if you wan to run
[system and integration tests][testing] you need to run a recent version of Docker in your
dev environment.


[testing]: agent_tests.md