# Troubleshooting

## Profiles not appearing in the UI

Check the host-profiler container logs:

```shell
kubectl logs <pod> -c host-profiler
```

Look for API key errors, endpoint errors, or config validation failures.

Also verify:

- `DD_SITE` matches your Datadog org's site.
- The API key secret is non-empty: `kubectl get secret datadog-secret -o jsonpath='{.data.api-key}' | base64 -d`

## No profiles for a specific process

- If `DD_SERVICE` is not set on a workload, profiles group under the interpreter name (e.g. `java`, `python`). Set `DD_SERVICE` on the workload to separate it.
- For compiled languages, function names do not resolve in stripped binaries. Upload debug symbols manually — see [Manually uploading debug symbols](README.md#manually-uploading-debug-symbols).
