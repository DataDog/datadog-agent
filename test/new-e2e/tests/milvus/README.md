# Milvus lab

This package provisions a small lab with:

- one AWS EC2 host with Docker,
- a standalone Milvus container exposing metrics on `localhost:9091`,
- a host-installed Datadog Agent with the `milvus.d` integration enabled,
- fakeintake disabled so the Agent sends metrics to the Datadog API.

## Provision and keep the lab

Fetch Datadog API credentials with `dd-auth`, map them to the e2e runner variables, then run the suite in keep-stack mode:

```bash
eval "$(dd-auth --domain datadoghq.com --output)"
export E2E_API_KEY="$DD_API_KEY"
export E2E_APP_KEY="$DD_APP_KEY"

dda inv new-e2e-tests.run \
  --targets=./tests/milvus \
  --run=TestMilvus \
  --keep-stack \
  --timeout=1h
```

`--keep-stack` leaves the Pulumi stack up after the validation tests pass. The test output includes the Pulumi stack name and session output directory; use the generated outputs to find the host address.

If you only want to provision the lab and skip validation tests, set init-only:

```bash
E2E_INIT_ONLY=true dda inv new-e2e-tests.run \
  --targets=./tests/milvus \
  --run=TestMilvus \
  --timeout=1h
```

`E2E_INIT_ONLY=true` also skips stack deletion.

## Useful commands on the host

```bash
sudo datadog-agent status
sudo datadog-agent status collector --json
curl -sf http://localhost:9091/metrics
docker ps
docker logs milvus
```

## Cleanup

Destroy the Pulumi stack when done. The exact stack name is printed in the run output. You can also use the e2e stack cleanup helpers under `dda inv -l | grep e2e`.
