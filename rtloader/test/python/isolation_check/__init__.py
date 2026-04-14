from datadog_checks.base.checks import AgentCheck

# Module-level global. In a single interpreter, all instances share this.
# With sub-interpreters, each instance gets its own copy of this module,
# so setting run_count in one check should NOT affect another.
run_count = 0


class IsolationCheck(AgentCheck):
    """Test check for verifying sub-interpreter isolation.

    Each call to run() increments the module-level run_count and returns
    its value as a string. If two instances share the same interpreter,
    the second run() will return "2". If they're isolated, both return "1".
    """
    def run(self):
        global run_count
        run_count += 1
        return str(run_count)


__version__ = '0.0.1'
