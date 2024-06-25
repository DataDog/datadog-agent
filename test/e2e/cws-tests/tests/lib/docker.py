import os
import tarfile
import tempfile

import docker
from retry.api import retry_call

from lib.const import SEC_AGENT_PATH
from lib.log import LogGetter


def is_container_running(container):
    container.reload()
    if container.status != "running":
        raise Exception


class DockerHelper(LogGetter):
    def __init__(self):
        self.client = docker.from_env()

        self.agent_container = None

    def start_cspm_agent(self, image, datadog_agent_config=None):
        volumes = [
            "/var/run/docker.sock:/var/run/docker.sock:ro",
            "/proc/:/host/proc/:ro",
            "/sys/fs/cgroup/:/host/sys/fs/cgroup:ro",
            "/etc/passwd:/etc/passwd:ro",
            "/etc/os-release:/host/etc/os-release:ro",
            "/:/host/root:ro",
        ]

        if datadog_agent_config:
            volumes.append(f"{datadog_agent_config}:/etc/datadog-agent/datadog.yaml")

        site = os.environ["DD_SITE"]
        api_key = os.environ["DD_API_KEY"]

        self.agent_container = self.client.containers.run(
            image,
            environment=[
                "DD_COMPLIANCE_CONFIG_ENABLED=true",
                "HOST_ROOT=/host/root",
                f"DD_SITE={site}",
                f"DD_API_KEY={api_key}",
            ],
            volumes=volumes,
            detach=True,
        )

        return self.agent_container

    def start_cws_agent(self, image, datadog_agent_config=None, system_probe_config=None):
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

        if datadog_agent_config:
            volumes.append(f"{datadog_agent_config}:/etc/datadog-agent/datadog.yaml")

        if system_probe_config:
            volumes.append(f"{system_probe_config}:/etc/datadog-agent/system-probe.yaml")

        site = os.environ["DD_SITE"]
        api_key = os.environ["DD_API_KEY"]

        self.agent_container = self.client.containers.run(
            image,
            cap_add=["SYS_ADMIN", "SYS_RESOURCE", "SYS_PTRACE", "NET_ADMIN", "IPC_LOCK"],
            security_opt=["apparmor:unconfined"],
            environment=[
                "DD_RUNTIME_SECURITY_CONFIG_ENABLED=true",
                "DD_SYSTEM_PROBE_ENABLED=true",
                "HOST_ROOT=/host/root",
                f"DD_SITE={site}",
                f"DD_API_KEY={api_key}",
            ],
            volumes=volumes,
            detach=True,
        )

        return self.agent_container

    def download_policies(self):
        command = SEC_AGENT_PATH + " runtime policy download"
        site = os.environ["DD_SITE"]
        api_key = os.environ["DD_API_KEY"]
        app_key = os.environ["DD_APP_KEY"]
        return self.agent_container.exec_run(
            command,
            stderr=False,
            stdout=True,
            stream=False,
            environment=[
                f"DD_SITE={site}",
                f"DD_API_KEY={api_key}",
                f"DD_APP_KEY={app_key}",
            ],
        )

    def push_policies(self, policies):
        temppolicy = tempfile.NamedTemporaryFile(prefix="e2e-policy-", mode="w", delete=False)
        temppolicy.write(policies)
        temppolicy.close()
        temppolicy_path = temppolicy.name
        self.cp_file(temppolicy_path, "/etc/datadog-agent/runtime-security.d/default.policy")
        os.remove(temppolicy_path)

    def cp_file(self, src, dst):
        tar = tarfile.open(src + '.tar', mode='w')
        try:
            tar.add(src)
        finally:
            tar.close()
        data = open(src + '.tar', 'rb').read()
        self.agent_container.put_archive("/tmp", data)
        self.agent_container.exec_run("mv /tmp/" + src + " " + dst)

    def reload_policies(self):
        self.agent_container.exec_run(SEC_AGENT_PATH + " runtime policy reload")

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
