# Datadog Fake Intake

Exposes a catch-all API for Datadog Agent POST requests.

## Requirements

- [Golang 1.19](https://go.dev/dl/)

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
DD_DD_URL: "http://localhost:8080"
```

### Locally

1. Run fakeintake

```bash
go run $DATADOG_ROOT/datadog-agent/test/fakeintake/app/main.go
```

2. Configure Datadog Agent to use fakeintake

```yaml
# datadog.yaml
DD_DD_URL: "http://localhost:8080"
```

## How to build

The `fakeintake` container is built by the `datadog-agent` CI and available at https://hub.docker.com/r/datadog/fakeintake/tags. Here are the instructions to build a container locally, in case of changes to `fakeintake`.

### 🐳 Docker, locally

1. `cd` to `fakeintake` root

```bash
cd $DATADOG_ROOT/datadog-agent/test/fakeintake
```

2. Build and run a new container image

```bash
docker compose up --force-recreate
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
curl ${SERVICE_IP}/fakeintake/payloads/?endpoint={post_endpoint_path}
```

Example:

```bash
curl ${SERVICE_IP}/fakeintake/payloads/?endpoint=/api/V2/series
```

#### Juniper Notebook

Play with fakeintake in a Juniper Notebook

```python
# POST payloads
import base64
import requests
import json

data = "totoro|25|owner:kiki"
response = requests.post("http://localhost:8080/api/v2/series", data)

json_content = response.content.decode('utf8')

print(json_content)

# GET payloads
import base64
import requests
import json

response = requests.get("http://localhost:8080/fakeintake/payloads/?endpoint=/api/v2/series")

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
