# Testing the Agent

The Agent has a good code coverage but unit tests validate each package in a
quick and incomplete way, specially because mocking with go is not always effective.
For this reason, the Agent test suite also includes _system tests_,
_integration tests_ and _E2E (End to End) tests_.

## Integration tests

Integration tests validates one of more functions using the go test framework -
the difference with unit tests is that these tests require more context to complete,
like a third party service up and running. Integration tests are run at every
commit through the CI so the following requirements must be met:

  * tests must be implemented using the `testing` package from the standard lib.
  * tests must work both when invoked locally and when invoked from the CI.
  * tests must work on any supported platform or skipped in a clean way.
  * execution time matters and it has to be as short as possible.


## E2E tests

### Kitchen

For tests that require a fully configured Agent up and running in specific and
repeatable environments there are E2E (End to End) tests that are executed using
Test Kitchen from Chef on the supported platforms.

### Kubernetes

There are some end to end tests executed on top of Kubernetes.

See the dedicated docs about it [here](../../test/e2e/README.md).


## System tests

System Tests are in between Unit/Integration and E2E tests. The Agent consists of
several moving parts running together and sometimes it's useful to validate how such
components interact with each other, something that might be tricky to achieve by
only testing single functions.

System tests cover any use case that doesn't fit an integration test, like executing
a special binary built using a subset of packages and validate specific operations,
answering simple questions like _is dogstatsd correctly forwarding metrics?_ or
_are the Python bindings working?_.

System Tests might contain Go code, Python or shell scripts but to ease maintenance
and keep the execution environment simple, it's preferable to keep the number of
external dependencies as low as possible.
