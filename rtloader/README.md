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
* Cmake version 3.15 or above
* Go compiler with `cgo` capabilities to run the tests

### Optional Requirements

* [libexecinfo](https://github.com/fam007e/libexecinfo) or [libbacktrace](https://github.com/ianlancetaylor/libbacktrace)

RTLoader can optionally show stack traces when a segfault happens, using `execinfo`, which is provided out of the box by the glibc.
When building with other libc, you can install the shared libraries `libexecinfo` or `libbacktrace` instead.
CMake should automatically pick it up so that you don't need to configure anything.
If it doesn't, you can explicitly tell it where to find those by setting `Backtrace_LIBRARY` and `Backtrace_INCLUDE_DIR` options, eg. `-DBacktrace_LIBRARY=/usr/lib/libexecinfo.so -DBacktrace_INCLUDE_DIR=/usr/include`.

## Build

RtLoader can be built using CMake. Run the configurator/generator first:

```sh
cmake .
```

Then just run `make` to build the project.

## Examples

- [Exposing Go functionality to Python](https://github.com/DataDog/datadog-agent/pull/4234)

## Test

Tests are written in Golang using `cgo`, run the testsuite from the root folder:
```sh
make -C test
```
