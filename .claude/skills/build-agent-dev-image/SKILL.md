---
name: build-agent-dev-image
description: Build a local dev Docker image of the Datadog Agent using Dockerfiles/dev/Dockerfile — compiles the local source tree with the same build environment as `dda env dev run` and layers the binary on top of the official agent image.
argument-hint: "[--name <image-name>] [--tag <tag>] [--agent-image <base-image>] [--extra-args <build-args>]"
allowed-tools: Bash(docker *)
disable-model-invocation: true
---

# Build Agent Dev Image

Build a dev Docker image from the local source tree using `Dockerfiles/dev/Dockerfile`.

The image compiles the local agent binary inside `datadog/agent-dev-env-linux` (the same
environment as `dda env dev run -- dda inv -- agent.build`) and layers the result on top of
the official `gcr.io/datadoghq/agent:latest` image.

## Arguments: $ARGUMENTS

Parse the arguments above and extract:
- `--name <image-name>` — Docker image name (default: `datadog/agent-dev`)
- `--tag <tag>` — Docker image tag (default: `local`)
- `--agent-image <base-image>` — Override the base agent image (default: `gcr.io/datadoghq/agent:latest`)
- `--extra-args <args>` — Extra flags forwarded to `dda inv agent.build` inside the build

## Instructions

1. **Build the image** from the repo root with the resolved values:
   ```bash
   docker buildx build \
     -t <name>:<tag> \
     -f Dockerfiles/dev/Dockerfile \
     [--build-arg AGENT_IMAGE=<agent-image>] \
     [--build-arg BUILD_EXTRA_ARGS=<extra-args>] \
     .
   ```
   Run with `run_in_background: true` — the full build takes ~3–5 minutes.

2. **Show the user** the full command before starting.

## Examples

```
/build-agent-dev-image
/build-agent-dev-image --tag my-feature
/build-agent-dev-image --name myrepo/agent-dev --tag pr-1234
/build-agent-dev-image --agent-image gcr.io/datadoghq/agent:7.65.0
/build-agent-dev-image --extra-args "--bundle-ebpf"
```
