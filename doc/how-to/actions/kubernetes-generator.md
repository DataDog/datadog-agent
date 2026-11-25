# Generate Kubernetes private actions

Generate typed Kubernetes handlers and entrypoints using the repository's pinned
SDK sources:

```sh
bazel run //tools/privateactionrunner/kubernetes-generator
bazel run //tools/privateactionrunner/kubernetes-generator -- --bundle=core
bazel run //tools/privateactionrunner/kubernetes-generator -- --check
```

Edit `tools/privateactionrunner/kubernetes-generator/config.yaml` to select
resources and API versions. Edit the templates to change generated handlers.
Handwritten `New…Handler` constructors are registered alongside generated actions.
Generation replaces and removes `*.gen.go` files only in selected bundles;
`--check` reports stale files without modifying them. Use `--out=/tmp/kubernetes-actions`
to inspect output separately.
