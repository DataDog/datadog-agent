from retry.api import retry_call


class LogGetter:
    def get_log(agent_name):
        pass


def _wait_agent_log(agent_name, log_getter, pattern):
    for line in log_getter.get_log(agent_name):
        if pattern in line:
            return
    raise LookupError("{} | {}".format(agent_name, pattern))


def wait_agent_log(agent_name, log_getter, pattern, tries=10, delay=5):
    return retry_call(_wait_agent_log, fargs=[agent_name, log_getter, pattern], tries=tries, delay=delay)
