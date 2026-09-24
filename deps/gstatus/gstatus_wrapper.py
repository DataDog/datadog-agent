#!/opt/datadog-agent/embedded/bin/python
"""gstatus wrapper — runs the upstream gstatus __main__.py via the agent's embedded Python.

This is a thin wrapper that invokes the upstream (unmodified) gstatus
package's __main__ module via runpy.

The upstream gstatus package (__main__.py, __init__.py) is shipped
inside the datadog_gstatus wheel and installed into site-packages.
"""

import runpy

runpy.run_module("gstatus", run_name="__main__")
