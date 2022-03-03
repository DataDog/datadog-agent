import os
import tarfile
from tempfile import TemporaryFile

from kubernetes import client, config
from kubernetes.stream import stream
from lib.log import LogGetter
from lib.const import SEC_AGENT_PATH

class KubernetesHelper(LogGetter):
    def __init__(self, namespace, in_cluster=False):
        if in_cluster:
            config.load_incluster_config()
        else:
            config.load_kube_config()

        self.api_client = client.CoreV1Api()

        self.namespace = namespace
        self.pod_name = None

    def select_pod_name(self, label_selector):
        resp = self.api_client.list_namespaced_pod(namespace=self.namespace, label_selector=label_selector)
        for i in resp.items:
            self.pod_name = i.metadata.name
            break
        LookupError(label_selector)

    def get_log(self, agent_name):
        log = self.api_client.read_namespaced_pod_log(
            name=self.pod_name, namespace=self.namespace, container=agent_name, follow=False, tail_lines=5000
        )

        return log.splitlines()

    def exec_command(self, container, command=None):
        if not command:
            command = []

        return stream(
            self.api_client.connect_post_namespaced_pod_exec,
            name=self.pod_name,
            namespace=self.namespace,
            container=container,
            command=command,
            stderr=True,
            stdin=False,
            stdout=True,
            tty=False,
        )

    def reload_policies(self):
        command = [SEC_AGENT_PATH, 'runtime', 'policy', 'reload']
        self.exec_command("security-agent", command=command)

    def download_policies(self):
        self.exec_command("security-agent", command=["mkdir", "-p", "/tmp/runtime-security.d"])
        site = os.environ["DD_SITE"]
        api_key = os.environ["DD_API_KEY"]
        app_key = os.environ["DD_APP_KEY"]
        command = ["/bin/bash", "-c",
                   "export DD_SITE=" + site +
                   " ; export DD_API_KEY=" + api_key +
                   " ; export DD_APP_KEY=" + app_key +
                   " ; " + SEC_AGENT_PATH + " runtime policy download --output-path " +
                   "/tmp/runtime-security.d/default.policy"]
        self.exec_command("security-agent", command=command)

    def retrieve_policies(self):
        command = ["cat", "/tmp/runtime-security.d/default.policy"]
        return self.exec_command("security-agent", command=command)
