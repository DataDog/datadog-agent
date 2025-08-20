# Set up your local machine manually

-----

These instructions are maintained on a best-effort basis. Prefer using the [developer environment](../tutorials/dev/env.md) instead.

## Prerequisites

Be sure that you already set up the [development requirements](required.md).

/// warning
Building the Agent for Windows requires using the [build image](https://github.com/DataDog/datadog-agent-buildimages/tree/main/windows). Setting up a local Windows environment is not officially supported.
///

## Build tools

CMake version 3.15 or later and a C++ compiler are required for building the Agent.

## Python

The Agent embeds a full-fledged CPython interpreter so it requires the development files to be available in the dev env. The Agent can embed Python 3, you will need development files for the version you want to support.

If you're on OSX/macOS, install <<<PYTHON_VERSION>>> with [Homebrew](https://brew.sh):

```
brew install python@<<<PYTHON_VERSION>>>
```

On Linux, depending on the distribution, you might need to explicitly install the development files, for example on Ubuntu:

```
sudo apt-get install python<<<PYTHON_VERSION>>>-dev
```

On Windows, install <<<PYTHON_VERSION>>> via the [official installer](https://www.python.org/downloads/) brings along all the development files needed:

/// warning
If you don't use one of the Python versions that are explicitly supported, you may have problems running the built Agent's Python checks, especially if using a virtualenv. At this time, only Python <<<PYTHON_VERSION>>> is confirmed to work as expected in the development environment.
///

### Python Dependencies

To protect and isolate your system-wide python installation, a python virtual environment is _highly_ recommended (though optional). It will help keep a self-contained development environment and ensure a clean system Python.

/// note
Due to the [way some virtual environments handle executable paths](https://bugs.python.org/issue22213) (e.g. `python -m venv`), not all virtual environment options will be able to run the built Agent correctly. At this time, the only confirmed virtual environment creator that is known for sure to work is `virtualenv`.
///

- Install the virtualenv module:
    ```
    python3 -m pip install virtualenv
    ```
- Create the virtual environment:
    ```
    virtualenv $GOPATH/src/github.com/DataDog/datadog-agent/venv
    ```
- [Activate the virtualenv](https://virtualenv.pypa.io/en/latest/user_guide.html#activators) (OS-dependent). This must be done for every new terminal before you start.

If using virtual environments when running the built Agent, you may need to override the built Agent's search path for Python check packages using the `PYTHONPATH` variable (your target path must have the [pre-requisite core integration packages installed](https://datadoghq.dev/integrations-core/setup/) though).

```sh
PYTHONPATH="./venv/lib/python<<<PYTHON_VERSION>>>/site-packages:$PYTHONPATH" ./agent run ...
```

See also some notes in [./checks](https://github.com/DataDog/datadog-agent/tree/main/docs/dev/checks) about running custom python checks.

## Golang

You must [install Golang](https://golang.org/doc/install) version `<<<GO_VERSION>>>` or later. Make sure that `$GOPATH/bin` is in your `$PATH`, otherwise [tooling](required.md#tooling) cannot use any additional tool it might need.

/// note
Versions of Golang that aren't an exact match to the version specified in our build images (see e.g. [here](https://github.com/DataDog/datadog-agent-buildimages/blob/c025473ee467ee6d884d532e4c12c7d982ce8fe1/circleci/Dockerfile#L43)) may not be able to build the agent and/or the [rtloader](https://github.com/DataDog/datadog-agent/tree/main/rtloader) binary properly.
///

### Installing tools

From the root of `datadog-agent`, run `dda inv install-tools` to install go tooling. This uses `go` to install the necessary dependencies.

## System or Embedded?

When working on the Agent codebase you can choose among two different ways to build the binary, informally named _System_ and _Embedded_ builds. For most contribution scenarios you should rely on the System build (the default) and use the Embedded one only for specific use cases. Let's explore the differences.

### System build

_System_ builds use your operating system's standard system libraries to satisfy the Agent's external dependencies. Since, for example, macOS 10.11 may provide a different version of Python than macOS 10.12, system builds on each of these platforms may produce different Agent binaries. If this doesn't matter to you—perhaps you just want to contribute a quick bugfix—do a System build; it's easier and faster than an Embedded build. System build is the default for all build and test tasks, so you don't need to configure anything there. But to make sure you have system copies of all the Agent's dependencies, skip the _Embedded build_ section below and read on to see how to install them via your usual package manager (apt, yum, brew, etc).

### Embedded build

_Embedded_ builds download specifically-versioned dependencies and compile them locally from sources. We run Embedded builds to create Datadog's official Agent releases (i.e. RPMs, debs, etc), and while you can run the same builds while developing locally, the process is as slow as it sounds. Hence, you should only use them when you care about reproducible builds. For example:

  * you want to build an agent binary that can be used as-is to replace the binary of an existing agent installation
  * some dependencies are not available on your system
  * you're working or debugging at a very low level: let's say you're adding a function to the Python bindings, you want to make sure you're using the exact same versions of Python as the official Agent packages

Embedded builds rely on [Omnibus](https://github.com/chef/omnibus) to download and build dependencies, so you need a recent `ruby` environment with `bundler` installed. See [how to build Agent packages with Omnibus](https://github.com/DataDog/datadog-agent/blob/main/docs/dev/agent_omnibus.md) for more details.

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

## Doxygen

We use [Doxygen](http://www.doxygen.nl) to generate the documentation for the `rtloader` part of the Agent.

To generate it (using the `dda inv rtloader.generate-doc` command), you'll need to have Doxygen installed on your system and available in your `$PATH`. You can compile and install Doxygen from source with the instructions available [here](http://www.doxygen.nl/manual/install.html). Alternatively, you can use already-compiled Doxygen binaries from [here](http://www.doxygen.nl/download.html).

To get the dependency graphs, you may also need to install the `dot` executable from [graphviz](http://www.graphviz.org/) and add it to your `$PATH`.

## Pre-commit hooks

It is optional but recommended to install `pre-commit` to run a number of checks done by the CI locally.

### Installation

To install it, run:

```sh
python3 -m pip install pre-commit
GOFLAGS=-buildvcs=false pre-commit install  # buildvcs avoids errors when getting go dependencies
```

The `shellcheck` pre-commit hook requires having the `shellcheck` binary installed and in your `$PATH`. To install it, run:

```sh
dda inv install-shellcheck --destination <path>
```

(by default, the shellcheck binary is installed in `/usr/local/bin`).

### Skipping `pre-commit`

If you want to skip `pre-commit` for a specific commit you can add `--no-verify` to the `git commit` command.

### Running `pre-commit` manually

If you want to run one of the checks manually, you can run `pre-commit run <check name>`.

You can run it on all files with the `--all-files` flag.

```sh
pre-commit run flake8 --all-files  # run flake8 on all files
```

See `pre-commit run --help` for further options.

## Setting up Visual Studio Code Dev Container

/// tip
Using the [developer environment](../tutorials/dev/env.md) approach is recommended.
///

[Microsoft Visual Studio Code](https://code.visualstudio.com/download) with the [devcontainer plugin](https://code.visualstudio.com/docs/remote/containers) allow to use a container as remote development environment in vscode. It simplify and isolate the dependencies needed to develop in this repository.

To configure the vscode editor to use a container as remote development environment you need to:

- Install the [devcontainer plugin](https://code.visualstudio.com/docs/remote/containers) and the [golang language plugin](https://code.visualstudio.com/docs/languages/go).
- Run the following command `dda inv vscode.setup-devcontainer --image "<image name>"`. This command will create the devcontainer configuration file `./devcontainer/devcontainer.json`.
- Start or restart your vscode editor.
- A pop-up should show-up to propose to "reopen in container" your workspace.
- The first start, it might propose you to install the golang plugin dependencies/tooling.
