# Milvus E2E scenario

End-to-end scenario that exercises the [Milvus integration][milvus-integration]
on a real Datadog backend (no fakeintake).

## What it provisions

A single AWS EC2 VM (Amazon Linux ECS AMI, Docker pre-installed) running:

| Component | How it's deployed |
|-----------|-------------------|
| Milvus standalone (`milvusdb/milvus:v2.4.13`) | Docker Compose, with `etcd` and `MinIO` deps |
| Traffic generator (`pymilvus` insert/search loop) | Docker Compose, mounts `testfixtures/traffic.py` |
| Datadog Agent | `agent.NewHostAgent` on the host, configured with `milvus.d/conf.yaml` |

Layout:

```
milvus/
├── milvus_test.go                              # Suite + assertions
├── provisioner.go                              # Custom typed Env (no FakeIntake)
├── testfixtures/
│   ├── docker-compose.milvus.yaml              # Milvus + etcd + MinIO + traffic
│   ├── milvus_integration.conf.yaml            # OpenMetrics endpoint + tags
│   └── traffic.py                              # pymilvus traffic generator
└── README.md
```

## Real intake (not fakeintake)

The custom `Env` in `provisioner.go` deliberately does **not** include a
`FakeIntake` component and never calls `agentparams.WithFakeintake`. With
those omitted, the Agent uses the runner-provided API key and the default
intake (`datadoghq.com`). Metrics, logs, and events therefore land in a real
Datadog org — by default the one whose API key is in
`DD_AGENT_API_KEY` / your e2e profile.

You need both an **API key** and an **app key** configured in your runner
profile (`~/.config/dda/.../config.yaml` or env: `E2E_API_KEY`,
`E2E_APP_KEY`). The app key is required for the test to query the metrics
backend.

## Per-run tagging

Each `TestMilvusE2E` invocation generates a random `testID` and:

* stamps `e2e_test_id:<testID>` into the integration config so the Milvus
  metric series carry the tag,
* also adds it as a global host tag via `agentparams.WithTags`,
* uses it as the Pulumi stack name so concurrent runs don't collide.

Assertions then query the Datadog metrics API with
`avg:milvus.proxy.num_collections{e2e_test_id:<testID>}`, scoping the test
to a single run.

## Running

```bash
dda inv new-e2e-tests.run --targets=./tests/agent-metric-pipelines/milvus/...
```

Add `-- -e2e.devMode` (or use `e2e.WithDevMode()` locally) to keep the VM
alive for SSH inspection after a failure.

## Reference

* Custom-env pattern this is based on:
  `test/new-e2e/examples/customenv_with_docker_app_test.go`
* Framework docs: `test/e2e-framework/AGENTS.md`
* Integration config example:
  <https://github.com/DataDog/integrations-core/tree/master/milvus>

[milvus-integration]: https://docs.datadoghq.com/integrations/milvus/
