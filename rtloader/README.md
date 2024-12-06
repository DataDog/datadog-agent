# Datadog Agent RtLoader

CPython embedding/extending backend with support for multiple Python versions.

## Concepts

RtLoader is a C++ wrapper around the CPython API with a C89-compatible public API
that can be used by foreign languages like Go. In order to provide support for
multiple Python versions, RtLoader fully abstracts Python in order to decouple client
applications and CPython. Which Python version to use can be decided at runtime,
RtLoader will `dlopen` the proper backend libraries accordingly.

## Architecture

### libdatadog-agent-rtloader

RtLoader exposes its C89-compatible API through `include/datadog_agent_rtloader.h`. By
using the `make3` function, the corresponding Python backend will
be loaded at runtime. Under the hood the library provides `RtLoader`, a C++ interface
that must be implemented by any supported backend, see `include/rtloader.h` for details.

### Two and Three

`libdatadog-agent-three` library provides Python3 support. Python2 isn't supported anymore.

### Common

The `common` folder contains C/C++ modules that are compiled into both
`libdatadog-agent-three` and `libdatadog-agent-two` to avoid code duplication.
Most of the code used to extend the embedded interpreter is there.

## Requirements

* C/C++ compiler
* Python 3.12.x development packages
* Cmake version 3.12 or above
* Go compiler with `cgo` capabilities to run the tests

## Build

RtLoader can be built using CMake. Run the configurator/generator first:

```sh
cmake .
```

Then just run `make` to build the project.

## Examples

- [Exposing Go functionality to Python](https://github.com/DataDog/datadog-agent/pull/4234)

## Demo

Examples about how to use RtLoader are provided in form of a C application under `demo`. The application expects to find a
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
