# Deploying Standalone: Using the OTel Operator

Complete the [prerequisites](README.md#prerequisites) before continuing.

If the OpenTelemetry Operator is not installed, follow the [official installation guide](https://opentelemetry.io/docs/kubernetes/operator/).

## Configure

### Image and DataDog site

In [`operator/collector.yaml`](operator/collector.yaml), update `spec.image` if you want a different image version, and set `DD_SITE` under `spec.env` to your Datadog site if you are not using `datadoghq.com`.

### OpenTelemetry collector configuration

The collector configuration lives in [`operator/collector.yaml`](operator/collector.yaml) under `spec.config` and can be configured like a regular collector.

## Deploy

```shell
kubectl apply -f standalone/operator/collector.yaml
```

On non-Cilium clusters:

```shell
kubectl apply -f standalone/network-policy.yaml
```

[`operator/collector.yaml`](operator/collector.yaml) contains three resources:

1. A `ClusterRole` granting the collector's service account access to the kubelet APIs needed for hostname resolution.
2. A `ClusterRoleBinding` attaching that role to the `dd-host-profiler-collector` service account the Operator creates.
3. The `OpenTelemetryCollector` CR defining the DaemonSet.

The Operator reconciles the CR and creates the DaemonSet.

### Seccomp

The collector is automatically configured to run under a seccomp profile. An init container copies the profile from the collector image to every node at pod startup. No manual steps required.

### AppArmor (optional)

Load [`apparmor-profile`](../apparmor-profile) on each node using your cluster's AppArmor provisioning mechanism, then update `securityContext` in [`operator/collector.yaml`](operator/collector.yaml):

```yaml
securityContext:
  # ... existing fields ...
  appArmorProfile:
    type: Localhost
    localhostProfile: dd-host-profiler
```

### Cilium (optional)

On clusters with Cilium, replace the standard network policy with the Cilium one to get FQDN-scoped egress enforcement:

```shell
kubectl delete -f standalone/network-policy.yaml
kubectl apply -f standalone/cilium-network-policy.yaml
```
