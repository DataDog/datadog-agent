import os
import tarfile
import tempfile

from kubernetes import client, config
from kubernetes.stream import stream
from lib.const import SEC_AGENT_PATH
from lib.log import LogGetter


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
            return
        raise LookupError(label_selector)

    def get_log(self, agent_name):
        log = self.api_client.read_namespaced_pod_log(
            name=self.pod_name, namespace=self.namespace, container=agent_name, follow=False, tail_lines=10000
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
            stderr=False,
            stdin=False,
            stdout=True,
            tty=False,
        )

    def reload_policies(self):
        command = [SEC_AGENT_PATH, 'runtime', 'policy', 'reload']
        self.exec_command("security-agent", command=command)

    def download_policies(self):
        site = os.environ["DD_SITE"]
        api_key = os.environ["DD_API_KEY"]
        app_key = os.environ["DD_APP_KEY"]
        command = [
            "/bin/bash",
            "-c",
            "export DD_SITE="
            + site
            + " ; export DD_API_KEY="
            + api_key
            + " ; export DD_APP_KEY="
            + app_key
            + " ; "
            + SEC_AGENT_PATH
            + " runtime policy download",
        ]
        return self.exec_command("security-agent", command=command)

    def push_policies(self, policies):
        temppolicy = tempfile.NamedTemporaryFile(prefix="e2e-policy-", mode="w", delete=False)
        temppolicy.write(policies)
        temppolicy.close()
        temppolicy_path = temppolicy.name
        self.exec_command("security-agent", command=["mkdir", "-p", "/tmp/runtime-security.d"])
        self.cp_to_agent("security-agent", temppolicy_path, "/tmp/runtime-security.d/downloaded.policy")
        os.remove(temppolicy_path)

    def cp_to_agent(self, agent_name, src_file, dst_file):
        command = ['tar', 'xvf', '-', '-C', '/tmp']
        resp = stream(
            self.api_client.connect_post_namespaced_pod_exec,
            name=self.pod_name,
            namespace=self.namespace,
            container=agent_name,
            command=command,
            stderr=True,
            stdin=True,
            stdout=True,
            tty=False,
            _preload_content=False,
        )

        with tempfile.TemporaryFile() as tar_buffer:
            with tarfile.open(fileobj=tar_buffer, mode='w') as tar:
                tar.add(src_file)

            tar_buffer.seek(0)
            commands = []
            commands.append(tar_buffer.read())

        while resp.is_open():
            resp.update(timeout=1)
            if commands:
                c = commands.pop(0)
                resp.write_stdin(c)
            else:
                break
            resp.close()

        dirname = os.path.dirname(dst_file)
        command = ['mkdir', '-p', dirname]
        self.exec_command(agent_name, command=command)

        command = ['mv', f'/tmp/{src_file}', dst_file]
        self.exec_command(agent_name, command=command)
