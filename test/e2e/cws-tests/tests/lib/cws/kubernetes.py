import os
import tarfile
from tempfile import TemporaryFile

from kubernetes import client, config
from kubernetes.stream import stream
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

        stream(
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

    def kill_agent(self, agent_name, signal):
        command = ['pkill', signal, agent_name]
        self.exec_command(agent_name, command=command)

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

        with TemporaryFile() as tar_buffer:
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
