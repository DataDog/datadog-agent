# Milvus Pulumi scenario

Deployable e2e-framework scenario that stands up a Milvus stack with
real traffic, a Datadog Agent reporting to a real Datadog org, and
Autodiscovery-driven Milvus integration. Wrapped as a `go test` so it
slots into the existing runner — but the test body is intentionally
empty: this is a *deploy*, not a *check*.

## What it deploys

Built on the stock `awsdocker.Provisioner` (`environments.DockerHost`):
a single AWS EC2 VM running a Docker Compose project that contains:

| Service          | Image                                  | Role                                                       |
|------------------|----------------------------------------|------------------------------------------------------------|
| `datadog-agent`  | (framework default agent image)        | Runs the Milvus check via container-label Autodiscovery.   |
| `milvus`         | `milvusdb/milvus:v2.4.13`              | Milvus standalone. Carries the Datadog AD labels.          |
| `etcd`           | `coreos/etcd:v3.5.5`                   | Milvus metadata store.                                     |
| `minio`          | `minio/minio:RELEASE.2023-03-20…`      | Milvus object store.                                       |
| `milvus-traffic` | `python:3.11-slim`                     | Installs `pymilvus`, runs insert / search / query forever. |

## Layout

```
milvus/
├── provisioner.go              # awsdocker.Provisioner(...) wiring
├── milvus_test.go              # Empty test that drives `pulumi up`
├── testfixtures/
│   ├── docker-compose.milvus.yaml  # Milvus + etcd + MinIO + traffic + AD labels
│   └── traffic.py                  # pymilvus traffic generator
└── README.md
```

## Framework primitives used

Per `test/e2e-framework/AGENTS.md`, the stock typed environment with
`With*` options is preferred over a custom Pulumi program:

* `awsdocker.Provisioner` → `environments.DockerHost`.
* `ec2docker.WithoutFakeIntake()` → Agent ships to the real intake.
* `dockeragentparams.WithExtraComposeManifest("milvus", …)` → splices the
  Milvus stack into the same compose project as the Agent so docker.sock
  Autodiscovery picks it up.
* `dockeragentparams.WithEnvironmentVariables(...)` → injects
  `DD_E2E_TEST_ID` (interpolated into AD labels) and `DD_MILVUS_TRAFFIC_B64`
  (the script, base64-encoded so the file is created inside the container
  at boot).
* `dockeragentparams.WithTags(...)` → host-level `DD_TAGS`.

The Milvus integration is configured purely via Datadog Autodiscovery
container labels on the milvus service — no `conf.d` file, no custom env.

## Real intake

Because `WithoutFakeIntake()` is set and `WithFakeintake(...)` is never
called, the Agent uses the default intake (`datadoghq.com`) with the API
key from the runner profile. Configure with `E2E_API_KEY` (and
`E2E_APP_KEY` if you want to query the backend from any future test).

## Deploying

This scenario uses the default e2e-framework AWS environment (`agent-sandbox`).
Run the standard local setup once, then authenticate with the generated AWS SSO
profile before deploying:

```bash
dda inv e2e.setup --account=agent-sandbox
aws-vault exec sso-agent-sandbox-account-admin -- aws sts get-caller-identity
```

The `scripts/` directory wraps the framework runner with sensible defaults
and uses `dd-auth` to populate the Datadog API key. From the milvus directory:

```bash
./scripts/up.sh        # provision the stack (keeps it alive)
./scripts/check.sh     # smoke-test the deployment (containers + agent + traffic)
./scripts/test.sh      # up.sh -> wait -> check.sh   (use --teardown to also destroy)
./scripts/down.sh      # destroy the stack
```

Each script sources `scripts/_lib.sh`, which runs `dd-auth --output`,
exports `DD_API_KEY` / `DD_APP_KEY` / `DD_SITE`, and re-publishes them under
the names the framework/provisioner read (`E2E_API_KEY`, `E2E_APP_KEY`,
`MILVUS_DD_SITE`). To authenticate against a non-default org, set
`DD_AUTH_DOMAIN` (native dd-auth env var) before calling the scripts:

```bash
DD_AUTH_DOMAIN=dddev.datadoghq.com ./scripts/up.sh
```

The provisioner sets `DD_SITE` on the agent container when
`MILVUS_DD_SITE` is non-empty, so the agent ships to the same org that
owns the key.

Overrideable env vars:

| Var                  | Default      | Effect                                                |
|----------------------|--------------|-------------------------------------------------------|
| `MILVUS_STACK_NAME`  | `milvus-dev` | Pulumi stack name; shared by all four scripts.        |
| `MILVUS_E2E_TEST_ID` | stack name   | Stamped into AD labels, `DD_TAGS`, host tags.         |
| `MILVUS_WAIT_SECS`   | `180`        | How long `test.sh` waits after `up.sh` before checking. |
| `DDA`                | `dda`        | Path to the `dda` binary (use for non-PATH installs). |

Underlying command (what `up.sh` runs):

```bash
E2E_DEV_MODE=true E2E_STACK_NAME=milvus-dev MILVUS_E2E_TEST_ID=milvus-dev \
  dda inv new-e2e-tests.run \
  --targets=./tests/agent-metric-pipelines/milvus \
  --run=^TestMilvusEnv$
```

After deploy, look in Datadog for metrics filtered by
`e2e_test_id:<id>` (e.g. `milvus.proxy.num_collections`,
`milvus.proxy.search_latency`, `milvus.proxy.req_count`).

## Inspecting on the VM

The runner prints the EC2 host once provisioning is done:

```bash
ssh <printed host>
sudo docker ps                                # datadog-agent, milvus-*, etc.
sudo docker logs -f milvus-traffic            # "iteration=N ok"
sudo docker exec -it datadog-agent agent status | sed -n '/milvus/,/^$/p'
sudo docker exec -it datadog-agent agent check milvus
```

## References

* Pattern this is based on:
  `test/new-e2e/examples/dockerenv_with_extra_compose_test.go`
* Framework docs: `test/e2e-framework/AGENTS.md`
* Integration docs: <https://docs.datadoghq.com/integrations/milvus/>
