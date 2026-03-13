# Kubernetes Autodiscovery and Pod Check Annotations

This document describes how the Datadog Agent uses Kubernetes pod annotations for integration (check) autodiscovery and how to avoid common pitfalls.

## Pod check annotations

Many customers use Kubernetes pod annotations to run checks. The annotation format is:

- **Annotation key**: `ad.datadoghq.com/<container_name>.checks`
- **Annotation value**: A **JSON object** that maps check names to their configuration (init_config, instances, etc.).

Example:

```yaml
annotations:
  ad.datadoghq.com/my-container.checks: |
    {
      "openmetrics": {
        "init_config": {},
        "instances": [
          {
            "openmetrics_endpoint": "http://%%host%%:8080/metrics"
          }
        ]
      }
    }
```

## Invalid JSON causes the check to fail

**Any incorrect or invalid JSON in the pod check annotation will cause the check to fail**, even if the pod is found via autodiscovery. The agent will not run the check when the annotation value is not valid JSON (e.g. missing comma, trailing comma, wrong quotes, or malformed structure).

Common mistakes include:

- Missing comma between `"init_config": {}` and `"instances": [...]`
- Trailing comma in arrays or objects (e.g. `"instances": [{},]`)
- Using a string for `init_config` instead of an object (e.g. `"init_config": "{}"` instead of `"init_config": {}`)

When the JSON is invalid, the agent does **not** run the check and there is no check instance to show in `agent status` for that integration. The error is reported in the **Configuration Errors** section of `agent status` (under the "Autodiscovery" / "Configuration Errors" block), so always check that section when a pod-annotated check does not appear.

## Validating annotation JSON

To validate pod check annotation JSON before applying it to a pod (or to troubleshoot failures), use the agent CLI:

```bash
# Validate JSON from a file
agent validate-pod-annotation /path/to/annotation.json

# Validate JSON from stdin (e.g. paste or pipe)
echo '{"openmetrics":{"init_config":{},"instances":[{"openmetrics_endpoint":"http://%%host%%:8080/metrics"}]}}' | agent validate-pod-annotation
```

If the JSON is invalid, the command prints the error and exits with code 1. If valid, it exits with code 0.

## Related documentation

- [Kubernetes and Integrations](https://docs.datadoghq.com/containers/kubernetes/integrations/) (Datadog docs) — setup and examples for annotations.
- Run `agent status` and look at **Configuration Errors** when a check discovered via annotations is not running.
