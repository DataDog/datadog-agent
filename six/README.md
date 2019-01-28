# Datadog Agent Six

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

### libdatadog-agent-three and libdatadog-agent-two

These libraries provide Python support for extending and embedding by linking
different versions of the CPython library.

## Requirements

* C/C++ compiler
* Python 2.7.x development packages
* Python 3.7.x development packages
* Cmake version 3.12 or above

## Build

Six can be built using CMake. Run the configurator/generator first:

```sh
ccmake .
```

Then just run `make` to build the project.

## Demo

Examples about how to use Six are provided in form of a C application under `demo`
and a Go application under `demo_go` that uses `cgo`.

## Test

```sh
make -C test
```
