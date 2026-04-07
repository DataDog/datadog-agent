// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

# Socket Contention Validation Notes

The socket contention module tracks socket lock addresses from socket lifecycle hooks,
classifies contention tracepoint events into low-cardinality socket buckets, and emits
metrics through the Agent-side `socket_contention` check.

## Validation

1. Run the probe package test:

```sh
dda inv system-probe.test -s -p ./pkg/collector/corechecks/ebpf/probe/socketcontention
```

2. Build the CO-RE object when the runtime C code changes:

```sh
bazel build //pkg/collector/corechecks/ebpf/c/runtime:socket-contention
```

3. Compile the Agent-side check package:

```sh
go test ./pkg/collector/corechecks/ebpf/socketcontention
```

4. For manual validation, enable `socket_contention.enabled`, start `system-probe`,
and query:

```sh
curl --unix-socket <sysprobe-socket> http://unix/modules/socket_contention/check
```

The endpoint should return a JSON array of contention buckets with:

- `object_kind`
- `socket_type`
- `family`
- `protocol`
- `lock_subtype`
- `cgroup_id`
- `flags`
- `count`
- `total_time_ns`
- `min_time_ns`
- `max_time_ns`

When contention is not classified as a registered socket lock, the bucket falls back
to `object_kind=unknown`.
