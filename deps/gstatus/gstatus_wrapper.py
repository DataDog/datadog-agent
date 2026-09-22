#!/opt/datadog-agent/embedded/bin/python
"""gstatus wrapper — runs the upstream gstatus __main__.py via the agent's embedded Python.

This is a thin wrapper that invokes the upstream (unmodified) gstatus
__main__.py from the agent's Python site-packages using runpy. It is not
a derivative work of the GPLv3 upstream code — it is a generic "run this
Python module" invocation.

The upstream __main__.py is shipped inside the datadog_gstatus wheel as
gstatus/__main__.py and installed into site-packages.
"""

import runpy

runpy.run_path(
    "/opt/datadog-agent/embedded/lib/python3.13/site-packages/gstatus/__main__.py",
    run_name="__main__",
)
