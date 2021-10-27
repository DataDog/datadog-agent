import docker
import os
from retry.api import retry_call

from lib.log import LogGetter


def is_container_running(container):
    container.reload()
    if container.status != "running":
        raise Exception


class DockerHelper(LogGetter):
    def __init__(self):
        self.client = docker.from_env()

        self.agent_container = None

    def start_agent(self, image, policy_filename=None, datadog_agent_config=None, system_probe_config=None):
        volumes = [
            "/var/run/docker.sock:/var/run/docker.sock:ro",
            "/proc/:/host/proc/:ro",
            "/sys/fs/cgroup/:/host/sys/fs/cgroup:ro",
            "/etc/passwd:/etc/passwd:ro",
            "/etc/group:/etc/group:ro",
            "/:/host/root:ro",
            "/sys/kernel/debug:/sys/kernel/debug",
            "/etc/os-release:/etc/os-release",
        ]

        if policy_filename:
            volumes.append(
                "{}:/etc/datadog-agent/runtime-security.d/default.policy".format(policy_filename),
            )

        if datadog_agent_config:
            volumes.append("{}:/etc/datadog-agent/datadog.yaml".format(datadog_agent_config))

        if system_probe_config:
            volumes.append("{}:/etc/datadog-agent/system-probe.yaml".format(system_probe_config))

        self.agent_container = self.client.containers.run(
            image,
            cap_add=["SYS_ADMIN", "SYS_RESOURCE", "SYS_PTRACE", "NET_ADMIN", "IPC_LOCK"],
            security_opt=["apparmor:unconfined"],
            environment=[
                "DD_RUNTIME_SECURITY_CONFIG_ENABLED=true",
                "DD_SYSTEM_PROBE_ENABLED=true",
                "HOST_ROOT=/host/root",
                "DD_SITE={}".format(os.environ["DD_SITE"]),
                "DD_API_KEY={}".format(os.environ["DD_API_KEY"]),
            ],
            volumes=volumes,
            detach=True,
        )

        return self.agent_container

    def wait_agent_container(self, tries=10, delay=5):
        return retry_call(is_container_running, fargs=[self.agent_container], tries=tries, delay=delay)

    def get_log(self, agent_name):
        log_prefix = None
        if agent_name == "security-agent":
            log_prefix = "SECURITY"
        elif agent_name == "system-probe":
            log_prefix = "SYS-PROBE"
        else:
            raise LookupError(agent_name)

        log = self.agent_container.logs(since=1).decode("utf-8")

        result = [line for line in log.splitlines() if log_prefix in line]
        if result:
            return result
        raise LookupError(agent_name)

    def close(self):
        if self.agent_container:
            self.agent_container.stop()
            self.agent_container.remove()

        self.client.close()
