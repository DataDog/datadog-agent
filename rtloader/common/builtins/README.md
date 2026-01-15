## Custom builtins.

This folder contains C modules that [extend](https://docs.python.org/3/extending/
extending.html#extending-python-with-c-or-c) the embedded CPython interpreter with
custom built-in modules. This allows Python integrations and custom checks to
import modules that only live in memory, in the main Agent process.

These C modules support Python 3.
