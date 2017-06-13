## package `py`

This package provides all the concrete types needed to load and run python checks.

In particular, it provides:

* implementations of the `Check` and `Loader` interfaces defined in the `check` package, for
python checks
* the bindings that python checks can use to fetch information from and submit data to the core agent

Before making modifications to the bindings, please make sure you understand the basics of the [Python
C API](https://docs.python.org/2/c-api/intro.html) usage. Also, make sure you have a good understanding
of the necessity and usage of the `stickyLock` struct (see inline documentation).
