from test_builder import TestCase


class K8CollectAllDocker(TestCase):
    name = "[Kubernetes&Docker] Agent collect all the logs from all the container from the Docker socket"

    def build(self, config):  # noqa: U100
        self.append(
            """
# Setup

Run an agent pod with and configure the agent to tail docker socket and set:

```
DD_LOGS_CONFIG_K8S_CONTAINER_USE_FILE=false
DD_LOGS_CONFIG_DOCKER_CONTAINER_USE_FILE=false
```

---
# Test

- All logs from all containers are collected
- All logs are properly tagged with container metadata
- Check that the `DD_CONTAINER_EXCLUDE` works properly (add `-e DD_CONTAINER_EXCLUDE=image:agent` when running the Datadog agent)

Example setup: https://github.com/DataDog/croissant-integration-resources/tree/master/logs-agent/qa/kubernetes/docker-socket

"""
        )


class K8DockerContainerLabels(TestCase):
    name = "[Kubernetes&Docker] Agent uses AD in Docker container labels"

    def build(self, config):  # noqa: U100
        self.append(
            """
# Setup
Logs agent AD configured using docker labels on K8s (labels set directly on the container via a Dockerfile, instead of pod annotations).

**Not supported yet** see https://datadoghq.atlassian.net/browse/AC-1530

------
# Test

- Collect all activated => Source and service are properly set
- Collect all disabled => Source and service are properly set and only this container is collected
- Check that processing rules are working in AD labels:  `com.datadoghq.ad.logs: '[{"source": "java", "service": "myapp", "log_processing_rules": [{"type": "multi_line", "name": "log_start_with_date", "pattern" : "\\d{4}\\-(0?[1-9]|1[012])\\-(0?[1-9]|[12][0-9]|3[01])"}]}]'``

"""
        )


class K8CollectAll(TestCase):
    name = "[Kubernetes] Agent collect all the logs with the Kubernetes integration"

    def build(self, config):  # noqa: U100
        self.append(
            """

# Setup

Deploy agent on K8s.

Deploy a pod using K8s AD annotations to enable logs collection: https://app.datadoghq.com/logs/onboarding/container

Check if annotation works on its own, and with container_collect_all enabled, exclusion, and few corner cases (see checklist)

Example setup: https://github.com/DataDog/croissant-integration-resources/tree/master/logs-agent/qa/kubernetes/ad-pod-annotations

- [k8s+docker] All logs from all containers are collected when `DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL=true` set.
- [k8s+docker] All logs are properly tagged with container metadata
- [k8s+docker] Check that the `DD_CONTAINER_EXCLUDE` works properly
- [k8s+docker] Check that the agent survives a corrupted log
- [k8s+docker] Only containers with tags are collected when `DD_LOGS_CONFIG_CONTAINER_COLLECT_ALL` is not set
- [k8s+docker] Check that lines longer than 16kB are supported and show up intact in the explorer

"""
        )


class K8PodAnnotation(TestCase):
    name = "[Kubernetes] Agent uses configuration in pod annotations"

    def build(self, config):  # noqa: U100
        self.append(
            """
# Setup
Example setup: https://github.com/DataDog/croissant-integration-resources/tree/master/logs-agent/qa/kubernetes/ad-pod-annotations

------
# Test

Check the following both for the logged pod starting before the agent, and for the logged pod starting after the agent.

- containerCollectAll: true => Source and service are properly set
- containerCollectAll: false => Source and service are properly set and only the annotated container is collected
- Check that processing rules are working in AD labels (the `logs-source` pod should have `one\ntwo\nthree` in its `agent stream-logs` output)

"""
        )


class K8FileTailingAnnotation(TestCase):
    name = "[Kubernetes] File tailing from AD/annotation"

    def build(self, config):  # noqa: U100
        self.append(
            """
# Setup
```
apiVersion: v1
kind: Pod
metadata:
  name: logapp
  annotations:
    ad.datadoghq.com/logapp.logs: '[{"type":"file","path":"/tmp/share/test.log"}, {"source":"redis","service":"redis"}]'
spec:
  containers:
  - name: logapp
    image: mingrammer/flog:latest
    imagePullPolicy: Always
    command: ["/bin/flog"]
    args: ["-l", "-d", "2"]
```

# Test

Check that the following combination are working as expected:

 * Single log config in the annotation with and without `container_collect_all`
 * Two log config : one for the container itself (no type in the config) and the other for the file
 * Single log config for the container

Example setup: https://github.com/DataDog/croissant-integration-resources/tree/master/logs-agent/qa/kubernetes/file-tailing-from-ad
"""
        )
