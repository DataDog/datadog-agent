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

When working on the Agent codebase you can choose among two different ways to
build the binary, informally named _System_ and _Embedded_ builds. For most
contribution scenarios you should rely on the System build (the default) and use
the Embedded one only for specific use cases, let's the differences in detail.

### System build

When performing a _System build_, libraries that could be found in your system
will be used to satisfy external dependencies, meaning that building the same
version of the Agent repo from two different environments (let's say macOS 10.11
and macOS 10.12) will produce slightly different binaries. If you are building
the agent only with the purpose of checking out how it works or to contribute a
patch, getting a different binary from the one Datadog ships with the official
packages won't be an issue most of the times. The tradeoff here is between build
reproducibility and ease of setup: this process relies on tooling available on
your system which you should be able to easily provide via the usual methods (apt,
yum, brew, etc) - you can see the other sections of this document if you need
details on how to setup them. If build reproducibility is not a requirement,
you should use a System build. System build is the default for all the build and
test tasks so there's no configuration steps to take.

### Embedded build

When performing an _Embedded build_, all the external dependencies needed by the
Agent are built locally from sources at specific versions. This is as slow as it
sounds so you should use Embedded builds only when you care about reproducible
builds, for example:

  * you want to build an agent binary that can be used as-is to replace the binary
    of an existing agent installation
  * some dependencies are not available on your system
  * you're working or debugging at a very low level: let's say you're adding a
    function to the Python bindings, you want to make sure you're using the exact
    same versions of Python as the official Agent packages

If you want to perform an Embedded build, you need to set the `use_system_libs`
boolean flag value to _false_, either exporting the env var `INVOKE_USE_SYSTEM_LIBS=false`,
changing the `invoke.yaml` file or passing the corresponding arg to the build and
test tasks, like `invoke build --use-system-libs=false`.

Embedded builds make use of Omnibus, so it's important to note that you also need
to setup `ruby` and `bundle`.

### Python

The Agent embeds a full-fledged CPython interpreter so it requires the development
files to be available in the dev env.

If you're on OSX/macOS, installing Python 2.7 with [Homebrew](https://brew.sh) will
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

On OSX/macOS with [Homebrew](https://brew.sh):
```
brew install net-snmp
```

On Windows TODO

On Ubuntu:
```
sudo apt-get install libsnmp-base libsnmp-dev snmp-mibs-downloader
```

**Please notice:** the package `snmp-mibs-downloader` is only available in the
`multiverse` Ubuntu repo and in `non-free` Debian repo. If you don't really
need to work/debug on the SNMP integration, you could just build the agent without
it (see [Building the Agent][building] for how to do it) and avoid the dependencies
setup efforts altogether.

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
[building]: agent_build.md