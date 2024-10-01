# Datadog Fake Intake

Exposes a catch-all API for Datadog Agent POST requests.

## Requirements

- [Golang 1.22](https://go.dev/dl/)

## How to run

### Docker

1. Pull the `fakeintake` container image from [the public registry](https://hub.docker.com/r/datadog/fakeintake/tags)

```bash
docker pull datadog/fakeintake
```

2. Start the docker container

```bash
docker run -i datadog/fakeintake
```

3. Configure Datadog Agent to use fake intake

```yaml
# datadog.yaml
DD_DD_URL: "http://localhost:80"
```

### Locally

1. Run fakeintake

```bash
go run $DATADOG_ROOT/datadog-agent/test/fakeintake/cmd/server/main.go
```

2. Configure Datadog Agent to use fakeintake

```yaml
# datadog.yaml
DD_DD_URL: "http://localhost:80"
```

## How to build

The `fakeintake` container is built by the `datadog-agent` CI and available at https://hub.docker.com/r/datadog/fakeintake/tags. Here are the instructions to build a container locally, in case of changes to `fakeintake`.

### Build fakeintake server and CLI

```bash
inv fakeintake.build
```

### 🐳 Docker, locally

1. `cd` to `fakeintake` root

```bash
cd $DATADOG_ROOT/datadog-agent/test/fakeintake
```

2. Build and run a new container image

```bash
docker compose up --force-recreate
```

## CLI

`fakeintakectl` is a CLI interacting with the fakeintake server.
It provides a sub-command for each method of the fakeintake client API and pretty-prints the output.

Ex.:

```bash
fakeintakectl --help
```
```
fakeintakectl is a CLI for interacting with fake intake servers.

Usage:
  fakeintakectl [command]

Available Commands:
  completion  Generate the autocompletion script for the specified shell
  filter      Filter logs or metrics
  flush       Flush the server and reset aggregators
  get
  help        Help about any command
  route-stats Get route stats

Flags:
  -h, --help         help for fakeintakectl
      --url string   fake intake server url

Use "fakeintakectl [command] --help" for more information about a command.
```

```bash
fakeintakectl --url http://internal-lenaic-eks-fakeintake-1048988617.us-east-1.elb.amazonaws.com route-stats
```
```
+-------------------+-------+
|       ROUTE       | COUNT |
+-------------------+-------+
| /api/v1/check_run |   542 |
| /api/v1/metadata  |    14 |
| /api/v2/logs      |  1095 |
| /api/v2/series    |   542 |
| /intake/          |    46 |
+-------------------+-------+
```

```bash
fakeintakectl --url http://internal-lenaic-eks-fakeintake-1048988617.us-east-1.elb.amazonaws.com get log-service names
```
```
agent
apps-http-client
apps-nginx-server
cluster-agent
dogstatsd
kube-proxy
redis
stress-ng
```

```bash
fakeintakectl --url http://internal-lenaic-eks-fakeintake-1048988617.us-east-1.elb.amazonaws.com filter logs --service agent
```
```
[
  {
    "message": "2023-12-06 14:52:13 UTC | TRACE | INFO | (pkg/trace/info/stats.go:91 in LogAndResetStats) | No data received",
    "status": "info",
    "timestamp": 1701874333582,
    "hostname": "i-0054118644b8ecfc4",
    "service": "agent",
    "ddsource": "agent",
    "ddtags": "filename:0.log,dirname:/var/log/pods/datadog_dda-linux-datadog-6frfx_ca4badce-510a-4402-bd7a-31bc3623e2d8/trace-agent,pod_phase:running,kube_qos:BestEffort,kube_namespace:datadog,image_name:gcr.io/datadoghq/agent,short_image:agent,kube_app_name:dda-linux-datadog,kube_app_instance:dda-linux,kube_app_component:agent,kube_container_name:trace-agent,kube_app_managed_by:Helm,kube_service:dda-linux-datadog,image_tag:7.49.0,image_id:gcr.io/datadoghq/agent@sha256:ba69302b2af6b2ac3372d76036527ccbb8fc9710e62d5381699e275080eaf49a,kube_ownerref_kind:daemonset,kube_daemon_set:dda-linux-datadog,git.repository_url:https://github.com/DataDog/datadog-agent,kube_ownerref_name:dda-linux-datadog,pod_name:dda-linux-datadog-6frfx,container_id:c55f1d1e514c7fc361bc032d6ecddd11a324f0c8fcc75ecaba87d11079d223a4,display_container_name:trace-agent_dda-linux-datadog-6frfx,container_name:trace-agent"
  },
  {
    "message": "2023-12-06 14:52:11 UTC | CORE | WARN | (pkg/collector/python/datadog_agent.go:131 in LogMessage) | disk:67cc0574430a16ba | (disk.py:136) | Unable to get disk metrics for /host/var/run/containerd/io.containerd.runtime.v2.task/k8s.io/cb2efc4ca4ed342261a31dfee6d54afa8ae9bc75ac695fe75e5680fbbf67de86/rootfs/host/proc/sys/fs/binfmt_misc: [Errno 40] Too many levels of symbolic links: '/host/var/run/containerd/io.containerd.runtime.v2.task/k8s.io/cb2efc4ca4ed342261a31dfee6d54afa8ae9bc75ac695fe75e5680fbbf67de86/rootfs/host/proc/sys/fs/binfmt_misc'. You can exclude this mountpoint in the settings if it is invalid.",
    "status": "info",
    "timestamp": 1701874332072,
    "hostname": "i-01d8952d17868da7f",
    "service": "agent",
    "ddsource": "agent",
    "ddtags": "filename:0.log,dirname:/var/log/pods/datadog_dda-linux-datadog-x97p4_d9807a31-2c73-421d-a443-8311828e243b/agent,kube_ownerref_kind:daemonset,kube_app_instance:dda-linux,kube_app_component:agent,kube_qos:BestEffort,short_image:agent,image_tag:7.49.0,kube_app_name:dda-linux-datadog,kube_service:dda-linux-datadog,kube_daemon_set:dda-linux-datadog,pod_phase:running,kube_namespace:datadog,kube_app_managed_by:Helm,kube_container_name:agent,image_name:gcr.io/datadoghq/agent,image_id:gcr.io/datadoghq/agent@sha256:ba69302b2af6b2ac3372d76036527ccbb8fc9710e62d5381699e275080eaf49a,git.repository_url:https://github.com/DataDog/datadog-agent,kube_ownerref_name:dda-linux-datadog,pod_name:dda-linux-datadog-x97p4,container_id:cb2efc4ca4ed342261a31dfee6d54afa8ae9bc75ac695fe75e5680fbbf67de86,display_container_name:agent_dda-linux-datadog-x97p4,container_name:agent"
  },
[…]
```

## API

### Get payloads

Returns all payloads submitted to a POST endpoint as byte arrays, encoded in base64.

#### Response

```golang
{
  "payloads": [][]byte
}
```

Example:

```json
{
  "payloads": [
    "dG90b3JvfDI1fG93bmVyOmtpa2k=" // use `b64.StdEncoding.DecodeString(str)` in golang or base64.b64decode(str) in python
  ]
}
```

#### curl

```bash
curl ${SERVICE_IP}/fakeintake/payloads?endpoint={post_endpoint_path}
```

Example:

```bash
curl ${SERVICE_IP}/fakeintake/payloads?endpoint=/api/V2/series
```

#### Jupyter Notebook

Play with fakeintake in a Jupyter Notebook

```python
# POST payloads
import base64
import requests
import json

data = "totoro|25|owner:kiki"
response = requests.post("http://localhost:80/api/v2/series", data)

json_content = response.content.decode('utf8')

print(json_content)

# GET payloads
import base64
import requests
import json

response = requests.get("http://localhost:80/fakeintake/payloads/?endpoint=/api/v2/series")

json_content = response.content.decode('utf8')

data = json.loads(json_content)

print(data)

print("==== Payloads ====")
for payload in data["payloads"]:
    print(base64.b64decode(payload))
```

## Development in VSCode

This is a sub-module within `datadog-agent`. VSCode will complain about the multiple `go.mod` files. While waiting for a full repo migration to go workspaces, create a go workspace file and add `test/fakeintake` to workspaces

```bash
go work init
go work use . ./test/fakeintake
```

> **Note** `go.work` file is currently ignored in `datadog-agent`
