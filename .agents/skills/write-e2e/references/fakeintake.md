# Asserting through fakeintake

Load before writing the first assertion, or when the payload you need has no client method yet.

fakeintake is the mock intake the agent ships to during a test. `test/fakeintake/AGENTS.md` is authoritative for the endpoint-to-aggregator mapping and for extending the server; this file covers what a test author needs.

Get the client with `s.Env().FakeIntake.Client()`, importing `"github.com/DataDog/datadog-agent/test/fakeintake/client"`.

## Finding the right method

| Looking for | Method |
|---|---|
| A metric by name | `FilterMetrics(name, opts...)` |
| A distribution | `FilterSketches(name, opts...)` |
| A check run | `FilterCheckRuns(name, opts...)` or `GetCheckRun(name)` |
| Logs for a service | `FilterLogs(service, opts...)` |
| An event | `FilterEvents(name, opts...)` |
| Traces or APM stats | `GetTraces()`, `GetAPMStats()` |
| Processes, containers | `GetProcesses()`, `GetContainers()`, `GetProcessDiscoveries()` |
| Network connections | `GetConnections()` |
| Container images, SBOMs, lifecycle | `GetContainerImageNames()`, `GetSBOMIDs()`, `GetContainerLifecycleEvents()` |
| Orchestrator resources | `GetOrchestratorResources(filter)`, `GetOrchestratorManifests()` |
| Agent health, telemetry, flare | `GetAgentHealth()`, `GetAgentTelemetryLogs()`, `GetLatestFlare()` |
| Host metadata or tags | `GetMetadata()`, `GetHostTags(hostname)`, `GetHosts()` |

When debugging an assertion that never fires, list what actually arrived: `GetMetricNames()`, `GetLogServiceNames()`, `GetCheckRunNames()`, `GetEventSources()`. Log the list from inside the failing callback — a mismatch is usually a name or tag typo, not a missing payload.

## Matchers

```go
client.WithTags[*aggregator.MetricSeries]([]string{"env:prod", "service:web"})
client.WithMatchingTags[*aggregator.MetricSeries]([]*regexp.Regexp{re})
client.WithMetricValueHigherThan(0)
client.WithMetricValueLowerThan(100)
client.WithMetricValueInRange(1, 10)
client.WithMessageContaining("totoro")   // logs
client.WithMessageMatching("^error: ")   // logs
client.WithAlertType(event.AlertTypeError)
```

The type parameter on `WithTags` is required and must match the payload type of the filter it is passed to.

## The polling idiom

```go
s.EventuallyWithT(func(c *assert.CollectT) {
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics("myfeature.points")
	require.NoError(c, err)
	assert.NotEmpty(c, metrics, "no myfeature.points received yet")
}, 2*time.Minute, 10*time.Second)
```

`require` on anything later code dereferences, so the iteration aborts instead of panicking on a nil result; `assert` for independent checks. Give each assertion a message naming what was expected — the failure output otherwise says only that a slice was empty.

If the callback also needs to drive an action (send a statsd point, append to a log file), do it inside the callback so it repeats every tick, and use `MustExecuteOn(c, ...)` rather than `MustExecute` so a transient SSH failure retries.

| Waiting for | Timeout / interval | Why |
|---|---|---|
| The first payload after the agent starts | `5*time.Minute, 10*time.Second` | Startup plus the first flush interval |
| A payload from an already-running agent | `2*time.Minute, 10*time.Second` | One or two flush intervals of slack |
| Process, service, or file state on the host | `1*time.Minute, 5*time.Second` | Local state, no network round trip |
| A check run right after a restart | `30*time.Second, 500*time.Millisecond` | Cheap to poll, expected almost immediately |

Longer is not safer: an overlong timeout turns a real failure into a slow failure and eats the job's budget.

## Proving a negative

Waiting for something never to arrive has no natural end. Establish a positive signal first, which proves at least one flush completed, then assert the absence synchronously:

```go
// A control metric that should pass the filter.
require.EventuallyWithT(s.T(), func(c *assert.CollectT) {
	s.sendGauge(allowedMetric, 1)
	s.sendGauge(blockedMetric, 1)
	metrics, err := s.Env().FakeIntake.Client().FilterMetrics(allowedMetric)
	assert.NoError(c, err)
	assert.NotEmpty(c, metrics)
}, 2*time.Minute, 5*time.Second)

// The pipeline has flushed, so the blocked metric would be here if it were forwarded.
metrics, err := s.Env().FakeIntake.Client().FilterMetrics(blockedMetric)
require.NoError(s.T(), err)
assert.Empty(s.T(), metrics, "filtered metric should not reach the intake")
```

## Resetting between phases

`FlushServerAndResetAggregators()` discards everything received so far, so a later assertion counts only new payloads. What it discards is gone — decide what you still need before calling it.

**Periodic payloads** — anything the agent re-emits every check or flush interval. When a phase restarts the agent or calls `UpdateEnv`, do that first, wait for the agent to be ready, then flush. Flushing earlier does not help: payloads produced under the old configuration keep arriving until the restart completes, so they land in the aggregator after the flush and the next assertion reads a mix of both configurations.

**Startup-only payloads** — anything emitted once when a component starts. Assert on these *before* any flush, because a flush deletes the only copy and the poll that follows can only time out. `tests/otel/otel-agent/dogtel_standalone_test.go` does this in `SetupSuite`, capturing the extension's liveness metric ahead of the flush that the first test performs.

When a suite has both, order the startup assertion first and treat the flush as the boundary between the two phases.

## One fakeintake per suite

Write assertions on the assumption that `FilterMetrics` returns your test's payloads — that is what a test author should be able to expect, and it is how tests in tree are written.

The framework does not reset the intake between tests, so a suite that needs to enforce that assumption resets it itself:

```go
func (s *mySuite) BeforeTest(suiteName, testName string) {
	s.BaseSuite.BeforeTest(suiteName, testName)
	require.NoError(s.T(), s.Env().FakeIntake.Client().FlushServerAndResetAggregators())
}
```

See `test/new-e2e/AGENTS.md` § "One fakeintake per suite".

## When there is no client method

Adding an aggregator for a new payload type is five steps, laid out in `test/fakeintake/AGENTS.md` § "Adding a new payload type". Touching `server/`, `aggregator/`, `api/`, `go.mod`, or the `Dockerfile` additionally requires bumping `test/fakeintake/version/VERSION` to a strictly higher integer in the same pull request, enforced by a CI gate that also runs in the merge queue; client-only and CLI-only changes need no bump.
