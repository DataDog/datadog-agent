import textwrap

from test_builder import TestCase


class ContainerTailJounald(TestCase):
    name = "[Journald] Agent collect docker logs through journald"

    def build(self, config):  # noqa: U100
        self.append(
            """ # Setup
Mount /etc/machine-id and use the following configuration

Create a `conf.yaml` at `$(pwd)/journald.d`

```
logs:
  - type: journald
    container_mode: true
```

```
docker run -d --name agent -e DD_API_KEY=... \\
  -e DD_LOGS_ENABLED=true \\
  -v $(pwd)/journald.d:/etc/datadog-agent/conf.d/journald.d:ro \\
  -v /var/run/docker.sock:/var/run/docker.sock \\
  -v /etc/machine-id:/etc/machine-id:ro \\
  -v /var/log/journal:/var/log/journal:ro \\
  datadog/agent:<AGENT_IMAGE>
```

----
# Test

- Once the agent is running, run `agent stream-logs`
- in another shell, `docker run --log-driver=journald --rm alpine:latest echo "hello world"`
- Search the logs stream for the message. Make sure the source and service tags are set correctly (should be the short image name)
"""
        )


class ContainerCollectAll(TestCase):
    name = "[Docker] Test Container Collect All"

    def build(self, config):  # noqa: U100
        self.append(
            """ # Setup
```
docker run -d -e DD_API_KEY=xxxxxxxxxxxxxx \\
     -e DD_LOGS_ENABLED=true \\
     -e DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL=true \\
     -v /var/run/docker.sock:/var/run/docker.sock:ro \\
     -v /proc/:/host/proc/:ro \\
     -v /opt/datadog-agent/run:/opt/datadog-agent/run:rw \\
     -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro \\
     datadog/agent:<AGENT_IMAGE>
```

## Generate logs
`docker run -it bfloerschddog/flog -l`

---
# Test

- All logs from all containers are collected
- All logs are properly tagged with container metadata
- Check that the `DD_CONTAINER_EXCLUDE` works properly (add `-e DD_CONTAINER_EXCLUDE="image:agent"` when running the Datadog agent)
- Long lines handling (>16kB)
    - `docker run -it bfloerschddog/flog -l -r 1 -b 17408` to generate logs > `16kb`
- `DD_LOGS_CONFIG_DOCKER_CONTAINER_USE_FILE=false` uses docker socket to collect logs

"""
        )


class AgentUsesAdLabels(TestCase):
    name = "[Docker] Agent uses AD in container labels"

    def build(self, config):  # noqa: U100
        self.append(
            """ # Setup
Run a container with an AD label:

```
docker run --name randomLogger -d \
--label com.datadoghq.ad.logs='[{"source": "randomlogger", "service": "mylogger"}]' \
chentex/random-logger:<AGENT_IMAGE>
```

------
# Test

- Collect all activated => Source and service are properly set
- Collect all disabled => Source and service are properly set and only this container is collected
- Check that processing rules are working in AD labels:  `com.datadoghq.ad.logs: '[{"source": "java", "service": "myapp", "log_processing_rules": [{"type": "multi_line", "name": "log_start_with_date", "pattern" : "\\d{4}\\-(0?[1-9]|1[012])\\-(0?[1-9]|[12][0-9]|3[01])"}]}]'``
- `DD_LOGS_CONFIG_DOCKER_CONTAINER_USE_FILE=false` uses docker socket to collect logs
"""
        )


class DockerMaxFile(TestCase):
    name = "[Docker] Agent collects logs with max-file=1"

    def build(self, config):  # noqa: U100
        self.append(
            """ # Setup
Run the agent locally on your computer with

```
logs_config:
  container_collect_all: true
```

## Case 1
Run a container with

```
docker run --log-driver json-file --log-opt max-size=10k --log-opt max-file=1 -d centos bash -c "i=1; while ((++i)); do echo \\$i hello alex; sleep 0.5; done"
```

## Test

- Check that you can still see logs after 1 minute (message `121 hello alex`) in the log explorer and none are missing

---

## Case 2
Run a container with

```
docker run --log-driver json-file -d centos bash -c "echo '1'; echo '2'; sleep 99999999"
```

## Test
- Check that after 2 minutes, you only see "1" and "2" in the log explorer

"""
        )


class DockerFileTailingAD(TestCase):
    name = "[Docker] File from volume tailing with AD / container label"

    def build(self, config):  # noqa: U100
        self.append(
            """ # Setup
With the docker listener & provider activated start a container with a file log config.

Run the Agent on a host while you use a container to generate log, in a file shared between the host and the container using a volume.

Only a file config:

```
$ mkdir -p /tmp/share
$ docker run -d --rm -v /tmp/share:/tmp/share -l com.datadoghq.ad.logs='[{"type":"file","path":"/tmp/share/test.log"}]' mingrammer/flog /bin/flog -d 5 -b 102  -l -o /tmp/share/test.log -t log
```

Both the container itself and a file:

```
$ mkdir -p /tmp/share
$ docker run -d --rm -v /tmp/share:/tmp/share  -l com.datadoghq.ad.logs='[{"type":"file","path":"/tmp/share/test.log"},{"type":"docker"}]' mingrammer/flog /bin/flog -d 5 -b 102  -l -o /tmp/share/test.log -t log
```

# Test
- Tailing a file from a docker label is working
- Tailing a file and the docker container is working
- Log coming from a file tailed thanks to a container label should bear all the tags related to the container

"""
        )


class DockerFileTail(TestCase):
    name = "[Docker] Tailing Docker container from file is supported"

    def build(self, config):  # noqa: U100
        self.append(
            """ # Setup
```
docker run -d -e DD_API_KEY=xxxxxxxxxxxxxx \
-e DD_LOGS_ENABLED=true \
-e DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL=true \
-e DD_EXTRA_LISTENERS=docker \
-v /var/run/docker.sock:/var/run/docker.sock:ro \
-v /proc/:/host/proc/:ro \
-v /opt/datadog-agent/run:/opt/datadog-agent/run:rw \
-v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro \
-v /var/lib/docker/containers:/var/lib/docker/containers:ro \
datadog/agent:<AGENT_IMAGE>
```

---
# Test

- All logs from all containers are collected from file and not from the docker socket (see `agent status` that will now show whether a container is tailed from the docker socket or it's log file)
- All logs are properly tagged with container metadata
- When the agent cannot reach /var/lib/docker/containers it should fallback on tailing from the docker socket
- Logs are properly tagged with container metadata

"""
        )


class PodmanFileTail(TestCase):
    name = "[Podman] Tailing podman containers from file is supported"

    def build(self, config):  # noqa: U100
        self.append(
            """ # Setup
```
Run the containerized agent in podman to enable AD to identify podman.  Note that podman can be installed on a mac (brew install podman) or in a VM.  Install at least podman-3.2.1, which is what the first customer using this functionality began with.

Note that we only support podman running as root, not as a user.

```
[core@localhost ~]$ API_KEY=...
[core@localhost ~]$ HOST=...
[core@localhost ~]$ IMAGE=...
[core@localhost ~]$ sudo podman run -d \
     --name dd-agent \
     -v /run/podman/podman.sock:/run/podman/podman.sock:ro \
     -v /var/lib/containers/storage:/var/lib/containers/storage:ro \
     -v /proc/:/host/proc/:ro \
     -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro \
     --log-driver=k8s-file \
     -e DD_API_KEY=$API_KEY \
     -e DD_LOG_LEVEL=debug \
     -e DOCKER_HOST=unix:///run/podman/podman.sock \
     --privileged \
     -e DD_LOGS_ENABLED=true \
     -e DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL=true \
     -e DD_HOSTNAME=$HOST \
     -e DD_LOGS_CONFIG_USE_PODMAN_LOGS=true \
     $IMAGE
...
[core@localhost ~]$ sudo podman run -d --log-driver=k8s-file bash -c 'while true; do date; sleep 1; done'
..
[core@localhost ~]$ sudo podman ps
CONTAINER ID  IMAGE                                                    COMMAND               CREATED         STATUS                       PORTS       NAMES
a7738ce94558  docker.io/datadog/agent-dev:dustin-mitchell-ac-1054-py3  /bin/entrypoint.s...  39 seconds ago  Up 38 seconds ago (healthy)              dd-agent
dd7ad06a44e6  docker.io/library/bash:latest                            -c while true; do...  4 seconds ago   Up 4 seconds ago                         inspiring_shaw
[core@localhost ~]$ sudo podman exec -ti dd-agent agent stream-logs
...
```

---
# Test

- All logs from podman containers are collected from file and not from the docker socket (see `agent status` that will now show whether a container is tailed from the docker socket or it's log file)
- All logs are properly tagged with container metadata

"""
        )


class PodmanSocketTail(TestCase):
    name = "[Podman] Tailing podman containers via API is supported"

    def build(self, config):  # noqa: U100
        self.append(
            """ # Setup

Run the containerized agent in podman to enable AD to identify podman.  Note that podman can be installed on a mac (brew install podman) or in a VM.  Install at least podman-3.3.1, which is the first version known to support this functionality.

Note that we only support podman running as root, not as a user.

```
[core@localhost ~]$ API_KEY=...
[core@localhost ~]$ HOST=...
[core@localhost ~]$ IMAGE=...
[core@localhost ~]$ sudo podman run -d \
     --name dd-agent \
     -v /run/podman/podman.sock:/run/podman/podman.sock:ro \
     -v /proc/:/host/proc/:ro \
     -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro \
     -e DD_API_KEY=$API_KEY \
     -e DD_LOG_LEVEL=debug \
     -e DOCKER_HOST=unix:///run/podman/podman.sock \
     --privileged \
     -e DD_LOGS_ENABLED=true \
     -e DD_LOGS_CONFIG_DOCKER_CONTAINER_USE_FILE=false \
     -e DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL=true \
     -e DD_HOSTNAME=$HOST \
     $IMAGE
...
[core@localhost ~]$ sudo podman run --log-driver=k8s-file -d bash -c 'while true; do date; sleep 1; done'
..
[core@localhost ~]$ sudo podman ps
CONTAINER ID  IMAGE                                                    COMMAND               CREATED         STATUS                       PORTS       NAMES
a7738ce94558  docker.io/datadog/agent-dev:dustin-mitchell-ac-1054-py3  /bin/entrypoint.s...  39 seconds ago  Up 38 seconds ago (healthy)              dd-agent
dd7ad06a44e6  docker.io/library/bash:latest                            -c while true; do...  4 seconds ago   Up 4 seconds ago                         inspiring_shaw
[core@localhost ~]$ sudo podman exec -ti dd-agent agent stream-logs
...
```

---
To check:

- All logs from podman containers are collected from the docker socket (see `agent status` that will now show whether a container is tailed from the docker socket or it's log file)
- All logs are properly tagged with container metadata
"""
        )


class ContainerScenario(TestCase):
    def __init__(self, k8s, cfgsource, cca, kcuf, dcuf):
        super().__init__()
        self.k8s = k8s
        self.cfgsource = cfgsource
        self.cca = cca
        self.kcuf = kcuf
        self.dcuf = dcuf

        self.name = "Container Scenario: "
        self.name += ('docker' if k8s == 'none' else 'k8s/' + k8s) + ', '
        self.name += f"config source={cfgsource}, "
        self.name += f"cca={cca}, "
        self.name += f"kcuf={kcuf}, "
        self.name += f"dcuf={dcuf}"

    def build(self, config):  # noqa: U100
        if self.k8s == 'docker':
            self.append(
                textwrap.dedent(
                    '''\
                # Set up Kubernetes

                Set up a Kubernetes environment that uses docker as its container runtime, such as
                `minikube --container-runtime=docker`.'''
                )
            )
        elif self.k8s == 'containerd':
            self.append(
                textwrap.dedent(
                    '''\
                # Set up Kubernetes

                Set up a Kubernetes environment that uses containerd as its container runtime, such as
                a newly-minted GKE environment.'''
                )
            )
        elif self.k8s == 'none':
            self.append(
                textwrap.dedent(
                    '''\
                # Set up Docker

                Set up a simple Docker environment, prepared to run the agent in a container with the
                necessary access to the host.'''
                )
            )

        if self.k8s != 'none':
            steps = textwrap.dedent(
                f'''\
                # Run the Agent

                If you choose to use Helm, include the following in your values:

                * `datadog.apiKey` = your API key
                * `datadog.logs.enabled` = `true`
                * `agents.image` = `7-rc`
                * `datadog.logs.containerCollectAll` = `{'true' if self.cca else 'false'}`
                * `datadog.logs.containerCollectUsingFiles` = `{'true' if self.kcuf else 'false'}`
                * `agents.containers.agent.env`:

                    ```
                    - name: "DD_LOGS_CONFIG_DOCKER_CONTAINER_USE_FILE"
                      value: "{'true' if self.dcuf else 'false'}"
                    ```
                '''
            )
            if self.k8s == 'docker':
                steps += textwrap.dedent(
                    '''\
                * `datadog.kubelet.tlsVerify` = `false` (for Minikube)
                * `datadog.criSocketPath` `/var/run/docker.sock` (for Minikube)'''
                )
            if self.cfgsource == 'file':
                steps += textwrap.dedent(
                    '''\
                * `datadog.confd`:

                    ```
                    "bash.yaml": |-
                      ad_identifiers:
                        - bash
                      logs:
                        service: TESTSVC
                        source: TESTSRC
                    ```
                '''
                )

            self.append(steps)
        else:
            self.append(
                textwrap.dedent(
                    f'''\
                # Run the Agent

                ```
                docker run -ti --rm --name dd-agent \\
                        --net=host \\
                        -e DOCKER_DD_AGENT= \\
                        -e DD_API_KEY=$DD_API_KEY \\
                        -e DD_LOGS_ENABLED=true \\
                        -e LOGS_CONFIG_CONTAINER_COLLECT_ALL={'true' if self.cca else 'false'} \\
                        -e LOGS_CONFIG_DOCKER_CONTAINER_USE_FILE={'true' if self.dcuf else 'false'} \\
                        -e LOGS_CONFIG_K8S_CONTAINER_USE_FILE={'true' if self.dcuf else 'false'} \\
                        -v /var/run/docker.sock:/var/run/docker.sock:ro \\
                        -v /proc/:/host/proc/:ro \\
                        -v /sys/fs/cgroup/:/host/sys/fs/cgroup:ro \\
                        -v /var/lib/docker/containers:/var/lib/docker/containers:ro \\
                        datadog/agent:7-rc
                ```
                '''
                )
            )

        self.append(
            textwrap.dedent(
                '''\
            # Start `agent stream-logs`

            Begin `agent stream-logs` before starting the tested container, so that you can see the totality
            of the logged data.'''
            )
        )

        if self.k8s == "none":
            labelargs = "\\"
            if self.cfgsource == "annotation":
                labelargs = '''--label com.datadoghq.ad.logs='[{"source": "TESTSRC", "service": "TESTSVC"}]' \\'''
            self.append(
                textwrap.dedent(
                    f'''\
                # Start the Container

                ```
                docker run -ti --rm \\
                    {labelargs}
                    bash -c 'i=0; while true; do echo "HELLO $i at" $(date); i=$((i+1)); sleep 1; done'
                ```
                '''
                )
            )
        else:
            annotargs = "\\"
            if self.cfgsource == "annotation":
                annotargs = (
                    '''--annotations ad.datadoghq.com/bash.logs='[{"source":"TESTSRC", "service":"TESTSVC"}]' \\'''
                )
            self.append(
                textwrap.dedent(
                    f'''\
                # Start the Container

                ```
                k run bash --image=bash \\
                    {annotargs}
                    --command -- bash -c 'i=0; while true; do echo "HELLO $i at" $(date); i=$((i+1)); sleep 1; done'
                ```
                '''
                )
            )

        if self.cfgsource == "none" and not self.cca:
            self.append(
                textwrap.dedent(
                    f'''\
                # Observe Nothing Logged

                Check `agent stream-logs`.  This {'container' if self.k8s ==
                'none' else 'pod'} has no configuration and container_collect_all
                is disabled, so none of the "HELLO" output should be logged.'''
                )
            )
        else:
            self.append(
                textwrap.dedent(
                    '''\
                # Observe Logging

                Check `agent stream-logs`:
                 * "HELLO" output logged
                 * First message included ("HELLO 0")
                 * First message has appropriate source and service (TESTSRC / TESTSVC if those were configured,
                   otherwise based on the image name)'''
                )
            )

        self.special_case_type_docker()
        self.special_case_empty_annotation()
        self.special_case_no_docker_file_access()

    def special_case_type_docker(self):
        if self.cfgsource == 'annotation':
            self.append(
                textwrap.dedent(
                    '''\
                # With `type:docker` in Annotation

                As an additional check, delete the test pod and create a new
                one with `"type": "docker"` added to the annotation.
                '''
                )
            )

    def special_case_empty_annotation(self):
        if self.cfgsource == 'annotation':
            self.append(
                textwrap.dedent(
                    '''\
                # With Empty Annotation

                As an additional check, delete the test pod and create a new
                one with `ad.datadoghq.com/bash.logs='[{}]'` as the annotation.
                The result should be that the pod is logged, but with default
                source and service based on the image name.
                '''
                )
            )

    def special_case_no_docker_file_access(self):
        if self.k8s == 'none' and self.dcuf:
            self.append(
                textwrap.dedent(
                    '''\
                # Without Access to Docker Directory

                As an additional check, restart the dd-agent container omitting
                the `-v /var/lib/docker/containers:/var/lib/docker/containers:ro`
                argument.  The agent should still log the container (using the
                Docker socket).
                '''
                )
            )
