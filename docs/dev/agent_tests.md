# Testing the Agent

The Agent has a good coverage of code but unit tests validate each package in a
quick but incomplete way, specially because in go mocking is not always effective.
For this reason the Agent test suite also includes _system tests_,
_integration tests_ and _E2E (End to End) tests_.

## Integration tests

Integration tests are executed using the go test framework. These tests require
more context than Unit tests, like a third party service up and running.
Integration tests are run at every commit through the CI.

## E2E tests

For tests that require a fully configured Agent up and running in specific and
repeatable environments there are E2E (End to End) tests that are executed using
Test Kitchen from Chef on the supported platforms.


## System tests

System Tests are in between Unit and E2E tests. The Agent consists of several
moving parts running together and sometimes it's useful to validate how such
components interact with each other, something that might be tricky to implement
only using a test framework.

System tests execute a special binary built using a subset of packages and validate
specific operations, answering simple questions like _is dogstatsd correctly
forwarding metrics_?

System Tests are supposed to be quite fast and are executed along with unit tests
at every commit through the CI.

To ease maintenance, it's preferable that system tests don't require external
dependencies so they should be written in either Go, Python or as shell scripts,
anything that can be invoked in the CI.
