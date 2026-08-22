# Remote Queries Private Action Runner and AgentSecure research

Date: 2026-07-02

This note captures the current Remote Queries POC harness shape versus the documented Datadog Private Actions installation paths.

Primary docs reviewed:

- <https://docs.datadoghq.com/actions/private_actions/>
- <https://docs.datadoghq.com/actions/private_actions/use_private_actions/?tab=docker>

## Summary

The Remote Queries AP/matrix harness does **not** currently use the documented Agent installer, standalone Docker runner, or a Docker Compose runner for Private Action Runner (PAR). It uses repository-local scripts that start source-built Agent and `privateactionrunner` binaries directly.

However, the real POC path does use the **existing PAR bundle system**. Remote Queries is added as a normal PAR bundle/action and dispatched by the existing PAR `WorkflowRunner`; it is not a new bundle framework.

AgentSecure setup is automatic in a normal Agent-based install. The harness only configures AgentSecure artifacts manually because it runs multiple local Agents and PARs side by side on one development host.

## Documented installation modes

The docs describe the Agent-based install as the recommended method:

- Linux or Windows host with Datadog Agent `7.77.0` or later.
- Kubernetes cluster with Datadog Operator `1.25.0` or later.
- Enable PAR through Agent configuration, for example:

```yaml
private_action_runner:
  enabled: true
```

The docs also describe standalone Docker/Kubernetes runner deployments as alternatives. Those run a standalone PAR image/deployment rather than the PAR binary packaged with the Agent.

## What the POC harness currently does

`test/remotequeries/ap-targeting-matrix-proof.sh` uses a custom local script flow:

1. Starts Postgres target fixtures with Docker Compose:
   - `test/remotequeries/targeting-matrix-compose.yaml`
   - This Compose file is for Postgres databases only; it is not a PAR Compose deployment.
2. Starts two source-built local Agent processes:

   ```bash
   "$AGENT_REPO/bin/agent/agent" run -c "$root"
   ```

3. Starts two source-built local PAR processes:

   ```bash
   "$AGENT_REPO/bin/privateactionrunner/privateactionrunner" run -c "$par_root"
   ```

4. Writes per-Agent and per-PAR configuration files itself under `$TMP_ROOT`.

So the harness is best described as:

- **Own script:** yes.
- **Documented manual installer:** no.
- **Standalone Docker PAR:** no.
- **Docker Compose PAR:** no.
- **Agent package/image service manager:** no.

It is runtime-shape-similar to the Agent-based install because the Agent and PAR processes share a config/IPC surface, but it does not use the documented installer or packaged service/container entrypoints.

## Agent-based PAR in main

Latest `main` includes Agent-package and Agent-container support for PAR. In those paths, the Agent distribution includes a separate `privateactionrunner` process/service:

- Linux service template:
  - `pkg/fleet/installer/packages/embedded/tmpl/datadog-agent-action.service.tmpl`
  - Runs `privateactionrunner run --cfgpath={{.EtcDir}}/datadog.yaml`.
- Agent container s6 service:
  - `Dockerfiles/agent/s6-services/privateactionrunner/run`
  - Runs `privateactionrunner run --cfgpath=/etc/datadog-agent/datadog.yaml`.

Latest `main` also includes Cluster Agent embedded PAR startup:

- `cmd/cluster-agent/subcommands/start/command.go`
- `comp/privateactionrunner/impl/privateactionrunner.go`

That Cluster Agent embedded PAR is not a drop-in replacement for the local matrix harness because the matrix relies on one runner/connection per local Agent and each runner calling its paired Agent's AgentSecure endpoint. A DCA-embedded runner would need routing from the DCA to the correct node Agent.

## Existing PAR bundling status

The real Remote Queries POC path uses the existing PAR bundle machinery:

- Bundle/action implementation:
  - `pkg/privateactionrunner/bundles/remoteaction/queries/entrypoint.go`
  - `pkg/privateactionrunner/bundles/remoteaction/queries/execute.go`
- FQN:

  ```text
  com.datadoghq.remoteaction.queries.execute
  ```

- Registered in the existing registry:
  - `pkg/privateactionrunner/bundles/registry.go`
- Dispatched by the existing runner:
  - `pkg/privateactionrunner/runners/workflow_runner.go`

This is a new bundle/action in the existing system, not a new bundling system.

One confusing exception is the dev-only `RemoteQueryPARHarness` in:

- `comp/remotequeries/impl/remote_query_par_poc.go`

That helper posts directly to an Agent IPC HTTP endpoint and does not exercise real PAR bundle dispatch. The AP matrix and standalone PAR process proofs should be treated as the authoritative bundle-based path.

## Standalone PAR to Agent communication

When PAR is standalone, the Agent does **not** talk to PAR. The direction is the opposite:

```text
Datadog/AP/fakeintake
  -> PAR polls/dequeues task
  -> PAR dispatches the task FQN through the existing PAR bundle registry
  -> Remote Queries PAR action calls local AgentSecure gRPC
  -> Agent executes through a loaded integration check and its credentials
  -> Agent streams results back to PAR
  -> PAR publishes task results back to AP/fakeintake
```

The Remote Queries action creates an AgentSecure client here:

- `pkg/privateactionrunner/bundles/remoteaction/queries/ipc_client.go`

It calls the Agent here:

- `pkg/privateactionrunner/bundles/remoteaction/queries/execute.go`
- `RemoteQueryExecuteStream(...)`

The Agent serves that RPC here:

- `comp/api/grpcserver/impl-agent/server.go`
- `comp/api/grpcserver/impl-agent/grpc.go`

## AgentSecure setup

AgentSecure is internal Agent IPC gRPC. The Agent IPC component creates or loads the IPC artifacts:

- `auth_token`
- `ipc_cert.pem` containing the IPC certificate and private key
- listener on `cmd_host:cmd_port`

Relevant code:

- `comp/core/ipc/impl/ipc.go`
  - `FetchOrCreateAuthToken(...)`
  - `FetchOrCreateIPCCert(...)`
- `pkg/api/security/security.go`
  - default auth token path resolution
- `pkg/api/security/cert/cert_getter.go`
  - default IPC cert path resolution
- `pkg/api/security/cert/cert_generator.go`
  - default certificate SANs include `localhost`, `127.0.0.1`, and `::1`
- `comp/api/grpcserver/impl-agent/grpc.go`
  - registers `AgentSecure`
  - uses TLS server config and requires a client certificate

For a standard Agent-based install, no extra AgentSecure manual steps are expected. The PAR service uses the same `datadog.yaml` and default IPC artifact paths as the Agent package/container.

For the local matrix harness, manual AgentSecure paths are required only because multiple Agents and PARs run on the same machine. Each Agent gets isolated values such as:

```yaml
cmd_host: 127.0.0.1
cmd_port: <unique port>
auth_token_file_path: <agent tmp dir>/run/auth_token
ipc_cert_file_path: <agent tmp dir>/run/ipc_cert.pem
```

Each paired PAR config points at the same Agent-specific values:

```yaml
cmd_host: 127.0.0.1
cmd_port: <same agent port>
auth_token_file_path: <same agent auth token>
ipc_cert_file_path: <same agent IPC cert>
```

For the current Remote Queries AgentSecure gRPC path, the client certificate is the important auth material:

- `NewDefaultBridgeClient()` loads the IPC cert with `cert.FetchIPCCert(...)`.
- `GetDDAgentSecureClient(...)` dials the Agent with that TLS config.
- The AgentSecure server requires client cert presence via `grpcutil.RequireClientCert` / `RequireClientCertStream`.

The `auth_token` remains part of general Agent IPC setup and is used by HTTP IPC paths, but the Remote Queries `RemoteQueryExecuteStream` gRPC call is authenticated by the IPC client certificate.

## Implications for next steps

If the goal is to stay close to documented Agent-based installation while preserving the local two-Agent matrix, the best next step is to keep the existing bundle/action path and reduce custom process wiring over time:

1. Keep `com.datadoghq.remoteaction.queries.execute` as a normal PAR bundle/action.
2. Prefer Agent-packaged PAR behavior where possible.
3. For the local harness, keep explicit `cmd_port` and IPC artifact paths because they are necessary to run multiple isolated Agents on one host.
4. If testing true standalone Docker PAR, expect extra custom work: the Docker image would need the Remote Queries bundle, network access to the Agent IPC port, and access to compatible IPC cert material.
5. If testing DCA-embedded PAR, add the Remote Queries bundle to the kubeapiserver registry and implement routing from DCA to the correct node Agent instead of assuming local Agent IPC.
