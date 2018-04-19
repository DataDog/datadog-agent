# Openshift installation and configuration instructions

Starting with version 6.1, the Datadog Agent supports monitoring OpenShift Origin and Enterprise clusters. Depending on your needs and the [security constraints](https://docs.openshift.org/latest/admin_guide/manage_scc.html) of your cluster, we support three deployment scenarios:

| Security Context Constraints   | [Restricted](#restricted-scc-operations) | [Host network](#host-network-scc-operations) | [Custom](#custom-datadog-scc-for-all-features) |
|--------------------------------|:----------:|:------------:|:------:|
| Kubernetes layer monitoring    | ✅         | ✅          | ✅     |
| Kubernetes-based Autodiscovery | ✅         | ✅          | ✅     |
| Dogstatsd intake               | 🔶         | ✅          | ✅     |
| APM trace intake               | 🔶         | ✅          | ✅     |
| Logs network intake            | 🔶         | ✅          | ✅     |
| Host network metrics           | ❌         | ❌          | ✅     |
| Docker layer monitoring        | ❌         | ❌          | ✅     |
| Container logs collection      | ❌         | ❌          | ✅     |
| Live Container monitoring      | ❌         | ❌          | ✅     |
| Live Process monitoring        | ❌         | ❌          | ✅     |

## General information

- You should first refer to the [common installation instructions](README.md), and its [Kubernetes section](README.md#Kubernetes)
- Our default configuration targets OpenShift 3.7.0 and later, as we rely on features and endpoints introduced in this version. [More installation steps](README.md#legacy-kubernetes-versions) are required for older versions.

## Restricted SCC operations

This mode does not require granting special permissions to the `datadog-agent` daemonset, other than the [RBAC](../manifests/rbac) permissions needed to access the kubelet and the apiserver. You can get started with this [kubelet-only template](../manifests/agent-kubelet-only.yaml).

Our recommended ingestion method for Dogstatsd, APM and logs is to bind our agent to a host port. This way, the target IP is constant and easily discoverable by your applications. As the default restricted OpenShift SCC does not allow to bind to host port, you can set the agent to listen on it's own IP, but you'll need to handle the discovery of that IP from your application.

We are currently working on a `sidecar` run mode, to enable running the agent in your application's pod for easier discoverability.

## Host network SCC operations

For easier intake, you can add the `allowHostPorts` permission to the pod (either via the standard `hostnetwork` or `hostaccess` SCC, or by creating your own). In this case, you can add the relevant port bindings in your pod specs:

```yaml
        ports:
          - containerPort: 8125
            name: dogstatsdport
            protocol: UDP
          - containerPort: 8126
            name: traceport
            protocol: TCP
```

## Custom Datadog SCC for all features

If SELinux is in permissive mode or disabled, you can simply enable the `hostaccess` SCC to benefit from all features. If SELinux is in enforcing mode, we recommend granting [the `spc_t` type](https://developers.redhat.com/blog/2014/11/06/introducing-a-super-privileged-container-concept/) to the datadog-agent pod. In order to easily deploy our agent, we created a [datadog-agent SCC](../manifests/openshift/scc.yaml) you can apply after [creating the datadog-agent service account](../manifests/rbac). It grants the following permissions:

- `allowHostPorts: true`: to bind Dogstatsd / APM / Logs intakes to the node's IP
- `allowHostPID: true`: to enable Origin Detection for Dogstatsd metrics submitted by Unix Socket
- `volumes: hostPath`: to access the Docker socket and the host's `proc` and `cgroup` folders, for metric collection
- `SELinux type: spc_t`: to access the Docker socket and all processes' `proc` and `cgroup` folders, for metric collection. You can read more about this type [in this Red Hat article](https://developers.redhat.com/blog/2014/11/06/introducing-a-super-privileged-container-concept/).

# Kubernetes-state metrics

[Kube-state-metrics](https://github.com/kubernetes/kube-state-metrics) does not collect metrics for OpenShift's DeploymentConfig objects. Although, you can get pod and container metrics tagging by deploying your kube-state-metrics pod with the following [Autodiscovery template in the pod annotations](https://docs.datadoghq.com/agent/autodiscovery/#template-source-kubernetes-pod-annotations).

```yaml
ad.datadoghq.com/kube-state-metrics.check_names: '["kubernetes_state"]'
ad.datadoghq.com/kube-state-metrics.init_configs: '[{}]'
ad.datadoghq.com/kube-state-metrics.instances: '[{"kube_state_url":"http://%%host%%:%%port%%/metrics","labels_mapper":{"namespace":"kube_namespace","label_deploymentconfig":"oshift_deployment_config","label_deployment":"oshift_deployment"},"label_joins":{"kube_pod_labels":{"label_to_match":"pod","labels_to_get":["label_deployment","label_deploymentconfig"]}}}]'
```

As OpenShift deployments create a Kubernetes replication controller with the same name, you can track you deployment's state via the `kubernetes_state.replicationcontroller.*` metrics.
