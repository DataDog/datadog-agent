# Datadog Fake Intake

Exposes a catch-all API for Datadog Agent POST requests.

## Requirements

- [Golang 1.18](https://go.dev/dl/)

## How to run

### Locally

1. cd to fakeintake root folder

```bash
cd ~/dd/datadog-agent/test/fakeintake
```

2. Build the fakeintake app

```bash
go install -o build/fakeintake app/main.go
```

3. Run the fakeintake

```bash
./build/fakeintake
```

4. Configure Datadog Agent to use fakeintake

```yaml
# datadog.yaml
DD_DD_URL: "http://localhost:8080"
```

### Docker

1. cd to fakeintake root folder

```bash
cd ~/dd/datadog-agent/test/fakeintake
```

2. Start the docker container

```bash
docker compose up
```

3. Configure Datadog Agent to use fake intake

```yaml
# datadog.yaml
DD_DD_URL: "http://localhost:8080"
```

## How to build

### 🐳 Docker, locally

1. Ensure you are using buildx `desktop-linux` driver

```bash
docker buildx create --use desktop-linux
```

2. Build a new multi-arch image using `buildx`. This will allow the container to run on both MacOS M1 (arm64) and Linux (amd64).

```bash
docker buildx build --push --platform linux/arm64/v8,linux/amd64 --tag <repo_name>/fakeintake:<tag> .
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
curl ${SERVICE_IP}/fake/payloads/{post_endpoint_path}
```

Example:

```bash
curl ${SERVICE_IP}/fake/payloads/api/V2/series
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

response = requests.get("http://localhost:8080/fake/payloads/api/v2/series")

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
