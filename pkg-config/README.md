## pkg-config modules

This directory contains the `pkg-config` module files of the dependencies of the project, by platform and
"build type" (i.e. using either system libraries or embedded libraries).

During a build, pkg-config will search for a module file in these directories, in this order:
1. `<platform>/embedded/<module_name>` (unless `USE_SYSTEM_LIBS` is passed to the rake build command)
2. `<platform>/system/<module_name>`
3. default pkg-config paths of the environment

The "embedded/" module files define the flags to compile and link against the libraries
provided in the datadog-agent package.

The "system/" files should only be present for dependencies that don't provide pkg-config files
for the platform. They define the flags to compile and link against the libraries provided by the system.

`cgo` uses `pkg-config` to determine which compiler and linker flags to use when the following directive
is present in the go source file:

```
// #cgo pkg-config <module_name>
```

(see https://golang.org/cmd/cgo/ for more details on `cgo` usage)
