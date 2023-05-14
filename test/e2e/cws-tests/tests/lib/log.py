from abc import ABC, abstractmethod

from retry.api import retry_call


class LogGetter(ABC):
    @abstractmethod
    def get_log(self, _agent_name):
        raise NotImplementedError()


def _wait_agent_log(agent_name, log_getter, pattern):
    lines = log_getter.get_log(agent_name)
    for line in lines:
        if pattern in line:
            return
    raise LookupError(f"{agent_name} | {pattern}")


def wait_agent_log(agent_name, log_getter, pattern, tries=10, delay=5):
    return retry_call(_wait_agent_log, fargs=[agent_name, log_getter, pattern], tries=tries, delay=delay)
