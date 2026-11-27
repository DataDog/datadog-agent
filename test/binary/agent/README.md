# Core Agent binary tests

Binary tests run the Agent we build as a program, on one machine, and check what it sends.
They are Bazel tests built on [rules_itest](https://github.com/hermeticbuild/rules_itest).

```sh
bazel test //test/binary/agent:agent_running_test
```

Bazel caches the result until the Agent, fakeintake, the configuration or the test changes.

## How a test is put together

`BUILD.bazel` declares three things:

- `itest_service` targets for the processes the test needs. `fakeintake` runs the fakeintake
  server on a free port and waits for its health endpoint. `agent` runs `//cmd/agent` with
  `agent.yaml` and sends to that fakeintake.
- A `dd_agent_go_test` holding the assertions. It is tagged `manual` because it only works once
  the services are up.
- A `service_test` that starts the services, runs the Go test, and stops the services.

Settings that depend on the run go in the service's `env`: ports through
`$${@@//label:service}` (or `$${@@//label:service.port_name}` for `named_ports`), writable paths
through `$${TMPDIR}`. Fixed settings go in `agent.yaml`.

The Go test reads the fakeintake port from the `ASSIGNED_PORTS` environment variable and queries
it with the fakeintake client, the same client the E2E tests use.

## Adding a test

To assert on something else the core Agent sends with the same setup, add a `Test*` function to
`agent_running_test.go`. To change the Agent's configuration, declare another `itest_service`
with its own configuration file and a `service_test` that uses it.

Run the test with `--test_output=streamed` to see the service logs. A test that waits on a
payload should fail with a clear message within a few minutes rather than hang until the Bazel
timeout.
