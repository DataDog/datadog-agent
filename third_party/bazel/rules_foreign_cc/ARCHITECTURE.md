# Rules Foreign CC - Architecture

In this file, we describe how we think about the architecture
of `rules_foreign_cc`. It's goal is to help contributors orient themselves
and to document code restrictions and assumptions.

In general, we try to follow the common standard defined by
https://docs.bazel.build/versions/master/skylark/deploying.html.

## //foreign_cc

This is the core package of the rules. It houses all rules which orchestrate
builds in foreign (non-Bazel) c++ build systems.

`//foreign_cc:defs.bzl` contains reexports of all core rules and should act
as the single entry point for consumers of this project.

`//foreign_cc/private` contains the implementation details of various rules
and tools provided by this project. The symbols and signatures within this
package should not be relied on and can change with no prior warning. Any
functionality here should be moved into `//foreign_cc` if users want to
interface with a stable API.

## //examples (@rules_foreign_cc_examples)

There are two primary types of examples, "Top Level" and "Third Party".
"Top level" can also be thought of as integration tests for the rules as they
should not contain any dependencies on code outside of the repo. "Third Party"
examples are examples of the rules in existing external projects. For more
detals on the examples, see
[examples/README.md](./examples/README.md#third-party).

## //test

This package contains various tests of rules, rules which do not compile C++
code. These tests can be thought of as unittests in mocked environments.

## //toolchains

Contains all toolchain information for supported build systems. There are a
set of types of toolchains which you'll find here.

1. `built_toolchains`
2. `prebuilt_toolchains`
3. `preinstalled_toolchains`

For details on these types and the implemented toolchains, please see
[`./toolchains/README.md`](./toolchains/README.md)
