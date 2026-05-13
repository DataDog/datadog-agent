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

The `scripts/` directory wraps the framework runner with sensible defaults
and uses `dd-auth` to populate the API key. From the milvus directory:

```bash
./scripts/bootstrap-integrations-dev.sh   # one-time: keypair + pulumi passphrase
./scripts/up.sh                            # provision the stack (keeps it alive)
./scripts/check.sh                         # smoke-test (containers + agent + traffic)
./scripts/test.sh                          # up.sh -> wait -> check.sh   (--teardown to destroy)
./scripts/down.sh                          # destroy the stack
```

### Targeting `agent-integrations-dev`

The scenario targets the **agent-integrations-dev** AWS account
(`030537971304`). Plumbing:

1. **First-class environment entry in the framework.** We added
   `agentIntegrationsDevDefault()` in
   `test/e2e-framework/resources/aws/environmentDefaults.go` with the
   account’s VPC, three private subnets, `common` security group, and
   AWS SDK profile. Pure `E2E_STACK_PARAMS` overrides aren’t sufficient
   because the framework’s `DefaultSubnets()` reads from the per-env
   defaults struct, not from Pulumi config. `test/new-e2e/go.mod` already
   has a `replace` directive pointing at the in-tree e2e-framework, so
   the patch takes effect with no version bump.
2. **Env selection at runtime.** `_lib.sh` exports
   `E2E_ENVIRONMENTS=aws/agent-integrations-dev` so the framework picks
   the new entry instead of the default `aws/agent-sandbox`.
3. **AMI override.** The AMI IDs in
   `test/e2e-framework/resources/aws/platforms.json` are private to
   agent-sandbox / agent-qa. `_lib.sh` adds two Pulumi config overrides
   (`ddinfra:osDescriptor=amazon-linux-ecs::x86_64`,
   `ddinfra:osImageIDUseLatest=true`) that force the framework’s
   `os_resolver.go` to look the AMI up via the *public* SSM parameter
   `/aws/service/ecs/optimized-ami/amazon-linux-2/recommended/image_id`,
   which is readable from any account.

Resource details (from `aws ec2 describe-*` on 2026-05-13):

| Where it lives | Value |
|---|---|
| `aws:profile` | `exec-sso-agent-integrations-dev-account-admin` |
| VPC | `vpc-07e6913338cbe8fea` (`agent-integrations-dev`) |
| Subnets | the three private subnets `subnet-0acb59fda8504f5bb` / `0d7bb3d71e68abcc2` / `0658d161c778e6168` |
| Security group | `sg-03d583f7425a802f7` (`common`) |
| Instance profile | _empty_ (no `ec2InstanceRole` in this account) |

The EC2 lands in a private subnet — outbound goes through the NAT gateway,
and inbound SSH works over the Datadog VPN through the VPC’s Transit
Gateway routes (10.0.0.0/8). You must be on the corporate VPN for
`check.sh`/`ssh` to reach the VM.

### One-time bootstrap

```bash
aws-vault login sso-agent-integrations-dev-account-admin
./scripts/bootstrap-integrations-dev.sh
```

The bootstrap script is idempotent. It creates (or imports) an EC2 keypair
named `e2e-agent-integrations-dev-$USER`, stores it under `~/.ssh/`, and
generates a Pulumi passphrase under `~/.config/dd-agent-milvus-lab/`. The
remaining `up.sh` / `check.sh` / `down.sh` calls read those artifacts.

> Note: `dda inv e2e.setup` is **not** run for this flow. That task is
> hard-coded for `agent-sandbox` (see `tasks/e2e_framework/setup/aws.py`,
> `AVAILABLE_AWS_ACCOUNTS`). The bootstrap script replaces it for our
> account.

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
