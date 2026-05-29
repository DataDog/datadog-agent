# Deploying Bundled: Using the Datadog Operator

## Prerequisite

Deploy the Datadog Agent using the Operator. See the [installation guide](https://app.datadoghq.com/fleet/install-agent/latest?platform=kubernetes).

## Deploy

Add the annotations to your existing `DatadogAgent` CR:

```shell
kubectl annotate datadogagent datadog \
  agent.datadoghq.com/host-profiler-enabled="true" \
  'experimental.agent.datadoghq.com/image-override-config={"host-profiler":{"name":"registry.datadoghq.com/ddot-ebpf:7.81.0-preview-host-profiler-1.0"}}' \
  -n <namespace>
```

Or add them directly to your manifest and re-apply:

```yaml
metadata:
  annotations:
    agent.datadoghq.com/host-profiler-enabled: "true"
    experimental.agent.datadoghq.com/image-override-config: |
      {"host-profiler": {"name": "registry.datadoghq.com/ddot-ebpf:7.81.0-preview-host-profiler-1.0"}}
```

The Operator rolls out a new DaemonSet revision adding the host-profiler container. Agent pods restart one node at a time.

The profiler will run fully privileged in this configuration as Operator does not yet apply security enforcements. If you wish to reduce privileges, see next section.

## Capabilities and seccomp

### Capabilities

Apply the provided patch to set the required capabilities on the host-profiler container:

```shell
kubectl patch datadogagent datadog -n <namespace> --patch-file bundled/operator/override.yaml --type merge
```

### Seccomp (recommended)

Provision the seccomp profile to each node through your cluster's node management tooling before deploying the host-profiler.

The profile is at `/etc/dd-host-profiler/seccomp.json` inside the image. Copy it to `/var/lib/kubelet/seccomp/host-profiler.json` on every node, then add `seccompProfile` to [`operator/override.yaml`](operator/override.yaml):

```yaml
      containers:
        host-profiler:
          securityContext:
            seccompProfile:
              type: Localhost
              localhostProfile: host-profiler.json
```

## AppArmor (optional)

Load [`apparmor-profile`](../apparmor-profile) on each node using your cluster's AppArmor provisioning mechanism, then set `appArmorProfileName` on the host-profiler container override:

```yaml
spec:
  override:
    nodeAgent:
      containers:
        host-profiler:
          appArmorProfileName: localhost/dd-host-profiler
```
