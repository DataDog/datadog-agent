# Datadog Fake Intake

Exposes a catch-all API for Datadog Agent POST requests.

## Requirements

* [Golang 1.19](https://go.dev/dl/)

## How to run

### Locally

1. cd to fake-intake root folder

```bash
cd ~/dd/datadog-agent/test/fake-intake
```

2. Build the fake-intake app

```bash
go build
```

3. Run the fake-intake

```bash
./fake-intake
```

4. Configure Datadog Agent to use fake intake

```yaml
# datadog.yaml
DD_DD_URL: "http://localhost:5000"
```

### Docker

1. cd to fake-intake root folder

```bash
cd ~/dd/datadog-agent/test/fake-intake
```

2. Start the docker container

```bash
docker compose up
```

3. Configure Datadog Agent to use fake intake

```yaml
# datadog.yaml
DD_DD_URL: "http://localhost:5000"
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

Play with fake-intake in a Juniper Notebook

```python
# POST payloads
import base64
import requests
import json

data = "totoro|25|owner:kiki"
response = requests.post("http://localhost:5000/api/v2/series", data)

json_content = response.content.decode('utf8')

print(json_content)

# GET payloads
import base64
import requests
import json

response = requests.get("http://localhost:5000/fake/payloads/api/v2/series")

json_content = response.content.decode('utf8')

data = json.loads(json_content)

print(data)

print("==== Payloads ====")
for payload in data["payloads"]:
    print(base64.b64decode(payload))
```
