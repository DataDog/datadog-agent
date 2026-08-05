# Troubleshooting

## Find the Host Profiler logs

Confirm that pods with the Host Profiler are running on the expected nodes:

```shell
kubectl get pods -n <NAMESPACE> -o wide
```

If you use a Datadog Agent deployment path, the Host Profiler runs as a sidecar in the Agent DaemonSet. If you use an OpenTelemetry deployment path, it runs in its own OpenTelemetry Collector DaemonSet.

Check the logs for the deployment path you used.

For Datadog Agent deployment paths, use the Host Profiler sidecar container:

```shell
kubectl logs -n <NAMESPACE> <DATADOG_AGENT_POD_NAME> -c host-profiler
```

For OpenTelemetry deployment paths, the pod has a single container:

```shell
kubectl logs -n <NAMESPACE> <POD_NAME>
```

Look for API key errors, endpoint errors, network errors, permission errors, or Collector configuration validation failures.

## Pod does not start

Inspect the pod events:

```shell
kubectl describe pod -n <NAMESPACE> <POD_NAME>
```

Common causes:

- **Image pull errors:** verify that the preview image tag in your values or manifests is correct and that the cluster can pull from `registry.datadoghq.com`.
- **Missing API key secret:** for the example OpenTelemetry deployment paths, make sure the `datadog-secret` Secret exists in the same namespace as the Host Profiler and contains the `api-key` key. To check that the key exists without displaying its value:

  ```shell
  kubectl get secret -n <NAMESPACE> datadog-secret \
    -o jsonpath='{.data.api-key}' | wc -c
  ```

  A value greater than `0` means the encoded key is present.

- **Cluster security policy:** host-wide eBPF profiling requires host-level access, including `hostPID: true`, host kernel mounts, and eBPF-related Linux capabilities. If your cluster uses Pod Security Admission, OPA Gatekeeper, Kyverno, or another admission controller, make sure it allows the settings from the deployment guide you are using. The Host Profiler does not run as a privileged container.
- **Seccomp or AppArmor profile errors:** in the OpenTelemetry deployment paths, an init container installs the seccomp profile automatically. If the pod is stuck in init, inspect the init container logs. In the Datadog Operator preview path, seccomp is optional and must be provisioned manually if you enable it. AppArmor is optional; if you enable it, make sure the profile is loaded on every node where the Host Profiler can run.
- **Unsupported environment:** this preview requires Kubernetes nodes that support host-level DaemonSets. Serverless, virtual-node, and restricted-node environments are not supported. See [Supported environments](README.md#supported-environments).

## Profiles do not appear in Datadog

Profiles usually appear on the [Datadog Profiler](https://app.datadoghq.com/profiling) page within a few minutes after the rollout completes.

If no profiles appear:

1. Confirm that the Host Profiler pods are running on the nodes you expect.
2. Check the Host Profiler logs for export or configuration errors.
3. Verify that `DD_SITE` matches your Datadog site. For example, use `datadoghq.com`, `datadoghq.eu`, or another supported [Datadog site](https://docs.datadoghq.com/getting_started/site/).
4. Verify that the Datadog API key is available to the Host Profiler. OpenTelemetry deployment paths read it from the configured secret. Datadog Agent deployment paths use the Agent configuration by default.
5. Make sure the cluster allows egress to `https://otlp.<DD_SITE>`. If you enabled NetworkPolicy or CiliumNetworkPolicy, confirm that it allows Datadog OTLP egress.
6. In the Datadog Profiler UI, check the selected time range, environment, service, and host filters.

## Profiles are missing or grouped under the wrong service

If profiles appear for some workloads but not for a specific process:

- Make sure the workload is running on a node where the Host Profiler DaemonSet is scheduled.
- Make sure the process runs long enough to be sampled and exported. Very short-lived processes may not appear immediately.
- Set `OTEL_SERVICE_NAME` or `DD_SERVICE` on the workload so profiles appear under the expected service. If neither environment variable is set, the Host Profiler infers the service name from the binary name, such as `java` or `python` for interpreted workloads.
- Set `DD_ENV` and `DD_VERSION` for richer filtering in the Datadog Profiler UI. Support for equivalent metadata from `OTEL_RESOURCE_ATTRIBUTES` is in progress.

## Function names are missing or hard to read

For compiled languages such as C, C++, Rust, and Go, debug symbols are required for readable function names.

The Host Profiler uploads debug symbols to Datadog when they are available locally. If your production binaries are stripped, upload symbols from your build artifacts separately. See [Do I need debug symbols?](faq.md#do-i-need-debug-symbols).
