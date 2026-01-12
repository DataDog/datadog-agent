 Troubleshooting Agent Memory Usage

The Agent process presents unusual challenges when it comes to memory profiling
and investigation. Multiple memory spaces, with various heaps coming from multiple
different runtimes, can make identifying memory issues tricky.

The Agent has three distinct memory spaces, each handled independently:

- [Go](go.md)
- [C/C++](c++.md)
- [Python](python.md)

There is tooling to dive deeper into each of these environments,
but having logic flow through the boundaries defined by these runtimes and
their memory management often confuses this tooling, or yields inaccurate
results. A good example of a tool that becomes difficult to use in this
environment is Valgrind. The problem is Valgrind will account for all
allocations in the Go and CPython spaces, and these being garbage collected
can make the reports a little hard to understand. You can also try to use a
supression file to supress some of the allocations in Python or Go, but it is
difficult to find a supression file.

This guide covers Go and Python have facilities for tracking and troubleshooting.
Datadog also offers some C/C++ facilities to help you track allocations.
