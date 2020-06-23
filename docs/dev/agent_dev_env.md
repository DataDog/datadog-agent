# Setting up your development environment

## Python

The Agent embeds a full-fledged CPython interpreter so it requires the
development files to be available in the dev env. The Agent can embed Python2
and/or Python3, you will need development files for all versions you want to
support.

If you're on OSX/macOS, installing Python 2.7 and/or 3.8 with [Homebrew](https://brew.sh)
brings along all the development files needed:
```
brew install python@2
brew install python@3
```

On Linux, depending on the distribution, you might need to explicitly install
the development files, for example on Ubuntu:
```
sudo apt-get install python2.7-dev
sudo apt-get install python2.3-dev
```

On Windows, install Python 2.7 and/or 3.8 via the [official installer](https://www.python.org/downloads/).

### Additional Windows Tools
You will also need the Visual Studio for [Visual Studio for Python installer](http://aka.ms/vcpython27)

Download the [gcc toolchain](http://win-builds.org/).
- From the graphical package manager, select and install the needed libraries, leave the default (select all) if you're unsure.
- Make sure to select x86_64.
- Add installation folder to the %PATH%.


## Invoke + Python Dependencies

[Invoke](http://www.pyinvoke.org/) is a task runner written in Python
that is extensively used in this project to orchestrate builds and test
runs.

Though you may install invoke in a variety of way we suggest you use
the provided [requirements](https://github.com/DataDog/datadog-agent/blob/master/requirements.txt)
file and `pip`:

```bash
pip install -r requirements.txt
```

This procedure ensures you not only get the correct version of invoke, but
also any additional python dependencies our development workflow may require,
at their expected versions.
It will also pull other handy development tools/deps (reno, or docker).

Tasks are usually parameterized and Invoke comes with some default values that
are used in the official build. Such values are listed in the `invoke.yaml`
file at the root of this repo and can be overridden by setting `INVOKE_*` env
variables (see Invoke docs for more details).


### Note

We don't want to pollute your system-wide python installation, so a python virtual
environment is recommended (though optional). It will help keep an isolated development
environment and ensure a clean system python.

- Install the virtualenv module:
```pip2 install virtualenv```
- Create the virtual environment:
```virtualenv $GOPATH/src/github.com/DataDog/datadog-agent/venv```
- Specify the path when building the agent:
```invoke agent.build --python-home-2=$GOPATH/src/github.com/DataDog/datadog-agent/venv```

If you are using python 3 instead (or switching between python versions), you can also
add `--python-home-3=<path>` pointing to a python3 virtual environment.

## Golang

You must install [go](https://golang.org/doc/install) version 1.11.5 or above. Make
sure that `$GOPATH/bin` is in your `$PATH` otherwise Invoke cannot use any
additional tool it might need.

## Installing dependencies

From the root of `datadog-agent`, run `invoke deps`. This will:

- Use `go` to install the necessary dependencies
- Use `git` to clone [integrations-core][integrations-core]
- Use `pip` to install [datadog_checks_base][datadog_checks_base]

If you already installed [datadog_checks_base][datadog_checks_base] in your desired
Python, you can do `invoke deps --no-checks` to prevent cloning and pip install. If
you are already doing development on [integrations-core][integrations-core], you
can specify a path to [integrations-core][integrations-core] using the `--core-dir`
option or `DD_CORE_DIR` environment variable to omit just the cloning step.

## System or Embedded?

When working on the Agent codebase you can choose among two different ways to
build the binary, informally named _System_ and _Embedded_ builds. For most
contribution scenarios you should rely on the System build (the default) and use
the Embedded one only for specific use cases. Let's explore the differences.

### System build

_System_ builds use your operating system's standard system libraries to satisfy
the Agent's external dependencies. Since, for example, macOS 10.11 may provide a
different version of Python than macOS 10.12, system builds on each of these
platforms may produce different Agent binaries. If this doesn't matter to
you—perhaps you just want to contribute a quick bugfix—do a System build; it's
easier and faster than an Embedded build. System build is the default for all
build and test tasks, so you don't need to configure anything there. But to make
sure you have system copies of all the Agent's dependencies, skip the
_Embedded build_ section below and read on to see how to install them via your
usual package manager (apt, yum, brew, etc).

### Embedded build

_Embedded_ builds download specifically-versioned dependencies and compile them
locally from sources. We run Embedded builds to create Datadog's official Agent
releases (i.e. RPMs, debs, etc), and while you can run the same builds while
developing locally, the process is as slow as it sounds. Hence, you should only
use them when you care about reproducible builds. For example:

  * you want to build an agent binary that can be used as-is to replace the binary
    of an existing agent installation
  * some dependencies are not available on your system
  * you're working or debugging at a very low level: let's say you're adding a
    function to the Python bindings, you want to make sure you're using the exact
    same versions of Python as the official Agent packages

Embedded builds rely on [Omnibus](https://github.com/chef/omnibus) to download
and build dependencies, so you need a recent `ruby` environment with `bundler`
installed. See [how to build Agent packages with Omnibus][agent-omnibus] for more
details.

If you want to perform an Embedded build, you need to set the `use_system_libs`
boolean flag value to _false_, either exporting the env var `INVOKE_USE_SYSTEM_LIBS=false`,
changing the `invoke.yaml` file or passing the corresponding arg to the build and
test tasks, like `invoke build --use-system-libs=false`.

### Systemd

The agent is able to collect systemd journal logs using a wrapper on the systemd utility library.

On Ubuntu/Debian:
```
sudo apt-get install libsystemd-dev
```

On Redhat/CentOS:
```
sudo yum install systemd-devel
```

## Docker

If you want to build a Docker image containing the Agent, or if you wan to run
[system and integration tests][testing] you need to run a recent version of Docker in your
dev environment.


[testing]: agent_tests.md
[building]: agent_build.md
[agent-omnibus]: agent_omnibus.md
[integrations-core]: https://github.com/DataDog/integrations-core
[datadog_checks_base]: https://github.com/DataDog/integrations-core/tree/master/datadog_checks_base

## Doxygen

We use [Doxygen](http://www.doxygen.nl/) to generate the documentation for the `rtloader` part of the Agent.

To generate it (using the `invoke rtloader.generate-doc` command), you'll need to have Doxygen installed on your system and available in your `$PATH`. You can compile and install Doxygen from source with the instructions available [here](http://www.doxygen.nl/manual/install.html).
Alternatively, you can use already-compiled Doxygen binaries from [here](http://www.doxygen.nl/download.html).

To get the dependency graphs, you may also need to install the `dot` executable from [graphviz](http://www.graphviz.org/) and add it to your `$PATH`.

## Pre-commit hooks

It is optional but recommended to install `pre-commit` to run a number of checks done by the CI locally.
To install it, run:

```sh
pip install pre-commit
pre-commit install
```

The `shellcheck` pre-commit hook requires having the `shellcheck` binary installed and in your `$PATH`.
To install it, run:

```sh
inv install-shellcheck --destination <path>
```

(by default, the shellcheck binary is installed in `/usr/local/bin`).
