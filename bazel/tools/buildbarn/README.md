# Local Buildbarn

A minimal [Buildbarn](https://github.com/buildbarn) for trying remote execution
locally, until CI has a full remote execution setup. It is a temporary dev tool
and will be removed then.

The main use case is building Linux-only parts of the Windows build (CPython,
OpenSSL) on a Linux worker while the rest still builds natively, e.g. from a
Windows machine without access to a shared remote execution cluster.

## What runs

| Service | Role |
|---|---|
| `storage` | `bb-storage`: REAPI frontend and local CAS/AC, on `:8980` |
| `scheduler` | `bb-scheduler`, with its UI on `http://localhost:7982` |
| `worker` | `bb-worker`, hardlinking build directories |
| `runner` | `bb_runner` inside the dda dev env image, without network access |

Actions run in the same image as `dda env dev`, so the worker environment
matches local builds. The worker architecture is that of the host
(arm64 on Apple silicon).

## Usage

Start it with podman (or Docker):

```sh
podman compose -f bazel/tools/buildbarn/compose.yaml up -d
```

Then add a config to `user.bazelrc`:

```
common:rbe-local --remote_executor=grpc://localhost:8980
common:rbe-local --remote_cache=grpc://localhost:8980
common:rbe-local --remote_default_exec_properties=OSFamily=linux # must match the worker platform properties
common:rbe-local --noremote_local_fallback # fail instead of silently building locally
common:rbe-local --strategy=remote,sandboxed # overrides common:linux --strategy=sandboxed
common:rbe-local --jobs=32
```

and build with `bazel build --config=rbe-local ...`.

From a container, `localhost` is not the host: attach the container to the
compose network and use the `storage` service name instead:

```sh
podman network connect dd-buildbarn_default <container>
bazel build --config=rbe-local --remote_executor=grpc://storage:8980 --remote_cache=grpc://storage:8980 ...
```

Environment variables:

- `BB_BIND`: address the ports are published on, `127.0.0.1` by default.
  Everything is unauthenticated, so prefer an SSH tunnel
  (`ssh -N -L 8980:127.0.0.1:8980 <host>`) over `BB_BIND=0.0.0.0`.
- `BB_CONCURRENCY`: number of concurrent actions, 8 by default.
- `BB_RUNNER_IMAGE`: image actions run in.

Stop it with `down`; add `-v` to also drop the cached blobs.

## Caveats

- With podman on macOS, containers sharing the dda cache volumes may need
  `--security-opt label=disable`, as SELinux labels of the podman VM otherwise
  deny access across containers.
- Tools that are only available for x86_64, like Wine's `wmc`, fail on an arm64
  worker.
