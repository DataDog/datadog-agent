# Datadog Agent Six

[![Build status](https://ci.appveyor.com/api/projects/status/325i4ry39gdfbfxh?svg=true)](https://ci.appveyor.com/project/Datadog/datadog-agent-six)
[![CircleCI](https://circleci.com/gh/DataDog/datadog-agent-six.svg?style=svg)](https://circleci.com/gh/DataDog/datadog-agent-six)

CPython embedding/extending backend with support for multiple Python versions.

## Concepts

Six is a C++ wrapper around the CPython API with a C89-compatible public API
that can be used by foreing languages like Go. In order to provide support for
multiple Python versions, Six fully abstracts Python in order to decouple client
applications and CPython. Which Python version to use can be decided at runtime,
Six will `dlopen` the proper backend libraries accordingly.

## Architecture

### libdatadog-agent-six

Six exposes its C89-compatible API through `include/datadog_agent_six.h`. By
using the `make2` and `make3` functions, the corresponding Python backend will
be loaded at runtime. Under the hood the library provides `Six`, a C++ interface
that must be implemented by any supported backend, see `include/six.h` for details.

### Two and Three

`libdatadog-agent-three` and `libdatadog-agent-two` libraries provide Python support
for extending and embedding by linking different versions of the CPython library.

### Common

The `common` folder contains C/C++ modules that are compiled into both
`libdatadog-agent-three` and `libdatadog-agent-two` to avoid code duplication.
Most of the code used to extend the embedded interpreter is there.

## Requirements

* C/C++ compiler
* Python 2.7.x development packages
* Python 3.7.x development packages
* Cmake version 3.12 or above
* Go compiler with `cgo` capabilities to run the tests

## Build

Six can be built using CMake. Run the configurator/generator first:

```sh
cmake .
```

Then just run `make` to build the project.

## Demo

Examples about how to use Six are provided in form of a C application under `demo`. The application expects to find a
few things installed in the Python env. To easy development, a virtualenv can be used: the base check
and the Directory check have to be installed before running the demo, if you have a local clone of `integrations-core`
that should be a matter of `pip install /path_to_integrations_core/datadog_checks_base` and
`pip install /path_to_integrations_core/directory`. Then depending on which Python version your virtualenv provides, run:

OSX:
```
DYLD_LIBRARY_PATH=./three:./two ./demo/demo 2 $VIRTUAL_ENV
```

Unix
```
LD_LIBRARY_PATH=./three:./two ./demo/demo 2 $VIRTUAL_ENV
```

or

OSX
```
DYLD_LIBRARY_PATH=./three:./two ./demo/demo 3 $VIRTUAL_ENV
```

Unix
```
LD_LIBRARY_PATH=./three:./two ./demo/demo 3 $VIRTUAL_ENV
```

## Test

Tests are written in Golang using `cgo`, run the testsuite from the root folder:
```sh
make -C test
```
