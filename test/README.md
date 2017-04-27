Test
====

This folder contains different tests for the project.

System Tests
============

Unit tests validate each Go packages in a quick but incomplete way. Some test
requires more context (proxy, external software ...). For those we have End to
End tests (E2E) using Test Kitchen.

In between we have System Tests. Those tests validate that all the packages
compiled together produce a viable binary capable of the most simple features
(for example: start the agent, collect at least one metric and send it to the backend).
System Tests are launched with unit tests on every commit.

System tests must not extend the project requirements and are therefore written
in Go, Python or Shell.
