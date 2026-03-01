# build-ddot-byoc

`build-ddot-byoc` (**B**ring **Y**our **O**wn **C**ollector) is a CLI tool that replaces the
`otel-agent` binary inside a Datadog Agent OCI package with a custom build and pushes the result
to a registry of your choice.

This allows you to run a customized OpenTelemetry Collector distribution alongside the Datadog
Agent while still using the standard Datadog installer for deployment and lifecycle management.

## Prerequisites

- Go 1.25+
- Registry credentials for both the source agent OCI package and the output registry (see [Authentication](#authentication) below).

## Build

```bash
cd tools/build-ddot-byoc
go build .
```

## Usage

```
build-ddot-byoc --agent-oci <url> --otel-agent <path> --output-oci <url> [--os <os>] [--arch <arch>]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `--agent-oci` | yes | — | OCI URL of the source Datadog Agent package |
| `--otel-agent` | yes | — | Local path to the custom `otel-agent` binary |
| `--output-oci` | yes | — | OCI URL to push the customized package to |
| `--os` | no | current OS | Target OS (`linux`, `windows`) |
| `--arch` | no | current arch | Target architecture (`amd64`, `arm64`) |

The `oci://` prefix on URLs is accepted but optional.

## Authentication

`build-ddot-byoc` uses the same authentication mechanism for both the source agent package and
the output registry. Select the method via the `REGISTRY_AUTH` environment variable:

| `REGISTRY_AUTH` | Method | Additional env vars |
|---|---|---|
| unset / `docker` | Docker config file / credential helpers (`~/.docker/config.json`) | — |
| `gcr` | Google Application Default Credentials (GCE, Workload Identity, `gcloud auth login`) | — |
| `password` | Static username and password | `REGISTRY_USERNAME`, `REGISTRY_PASSWORD` |

### Examples

**Docker config (default)** — works automatically if you have run `docker login` or have a
credential helper configured:
```bash
./build-ddot-byoc --agent-oci ... --otel-agent ... --output-oci ...
```

**Google Cloud (GCR / Artifact Registry)**:
```bash
REGISTRY_AUTH=gcr ./build-ddot-byoc --agent-oci ... --otel-agent ... --output-oci ...
```

**Username / password**:
```bash
REGISTRY_AUTH=password \
REGISTRY_USERNAME=myuser \
REGISTRY_PASSWORD=mypass \
  ./build-ddot-byoc --agent-oci ... --otel-agent ... --output-oci ...
```

## Example

```bash
./build-ddot-byoc \
  --agent-oci  registry.datadoghq.com/agent:7.78.0 \
  --otel-agent ./bin/otel-agent/otel-agent \
  --output-oci my-registry/my-project/ddot-byoc/agent:custom \
  --os   linux \
  --arch amd64
```

### Building a custom otel-agent

Use the `otel-agent.build` invoke task from the root of the `datadog-agent` repository:

```bash
dda inv otel-agent.build --byoc
# binary is written to bin/otel-agent/otel-agent
```

For Windows cross-compilation from Linux (requires `gcc-mingw-w64`):

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
  CXX=x86_64-w64-mingw32-g++ CC=x86_64-w64-mingw32-gcc \
  dda inv otel-agent.build --byoc
# binary is written to bin/otel-agent/otel-agent.exe
```

### Installing on a host with the Datadog installer

```bash
sudo datadog-installer install oci://my-registry/my-project/ddot-byoc/agent:custom
```

The installer extracts the ddot extension layer to
`/opt/datadog-packages/datadog-agent/<version>/extensions/ddot/`.
The custom `otel-agent` binary ends up at:

```
/opt/datadog-packages/datadog-agent/<version>/extensions/ddot/embedded/bin/otel-agent
```
