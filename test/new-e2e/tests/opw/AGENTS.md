# OPW E2E tests

End-to-end tests for the Agent's Observability Pipelines Worker (OPW) log
forwarder path — config keys rooted at `observability_pipelines_worker.logs`.

## Current coverage

- `opw_host_tags_test.go` — exercises the
  `observability_pipelines_worker.logs.send_host_tags` feature: asserts that
  user-configured host tags are appended to each log's `ddtags` when enabled
  and absent when disabled.

## How these tests work

OPW is a mock target here: the Agent's OPW logs URL is pointed at the
provisioned fakeintake instance, which accepts the Agent's HTTP/JSON log
format on `/api/v2/logs` just like the real Datadog intake. Tests read back
payloads via `FakeIntake.Client().FilterLogs(...)`.

The fakeintake URL is only known after provisioning, so tests start with an
empty provisioner and then call `UpdateEnv(...)` with the rendered Agent
config. See `orchestrator/k8s_api_key_test.go` for a similar pattern.

## Adding a new OPW test

1. Use `awshost.Provisioner` + `e2e.BaseSuite[environments.Host]`.
2. In each test method, call `UpdateEnv` with `agentparams.WithAgentConfig(...)`
   rendered against `s.Env().FakeIntake.URL`.
3. Assert fakeintake payloads with `EventuallyWithT`.

See `test/e2e-framework/AGENTS.md` and `test/fakeintake/AGENTS.md` for
framework and intake-mock docs respectively.
