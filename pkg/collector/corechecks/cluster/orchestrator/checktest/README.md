# Orchestrator check test suite

`checktest` tests the Cluster Agent orchestrator check as a whole.
The real check runs against a fake Kubernetes API server that serves an object of every resource the check collects, and each test compares everything the check sends with the full payloads that it expects.

The suite does not test the network, the intake or the backend: it checks what the check passes to its sender.
The processor and transformer tests check the fields of each model.

## Layout

```
checktest/
├── check_test.go   TestOrchestratorCheck: runs every registered group
├── cases/          the tests, one file for each area, each registered in init
└── suite/          the harness: the runner, the environment, the fake cluster and the expected payloads
```

`cases` only uses `suite`, and `suite` only uses the `*ForTest` hooks of the check in `../testing.go`.

## Concepts

```
Test = Setup + Steps

Setup ──creates──▶ Env ◀── each Step: change the Env, run the check, compare what it sent
```

- A **Test** is a name, a Setup and a list of Steps.
- A **Setup** is how the check is deployed and configured: the deployment mode (leader, follower or cluster check runner), the collector selection, the API versions and the undiscovered resources. The zero value is the Cluster Agent leader with every collector.
- An **Env** is the environment of one test: the configured check, the fake cluster with an object of every resource, the mock tagger and workloadmeta, and the Objects of the cluster in their current state. Each test gets a fresh Env.
- A **Step** has a `Change` to the Env, and an `Expect` that returns the payloads the check must send after it.

A test that must hold in several setups is listed once for each setup, with a shared list of steps, as in `cases/lifecycle.go`.

## How a step runs

1. The step applies its `Change`, for example `suite.DeleteAll(suite.Collected)`. Changes that delete objects wait until the check has queued their deletions, so the next run cannot miss them.
2. It calls `Expect`, and adds the cluster payload when the check leads and nodes were added or deleted since it last sent the cluster.
3. It runs the check until every expected payload has arrived, for up to 5 seconds, then runs it once more so that late duplicates show up.
4. It compares everything sent with the expected payloads with one `cmp.Diff`. A missing, extra, duplicate or different payload fails the step.
5. It checks that no payload contains the secret values of the fixtures.

## Step vocabulary

`suite/steps.go` names the common parts of a step:

| Kind | Names |
|---|---|
| Object sets | `Collected`, `Originals`, `Copies`, `CustomResources` |
| Changes | `UpdateAll`, `CopyAll`, `DeleteAll`, `SetLeader`, `FailDiscovery` |
| Expectations | `Live`, `Deleted`, `NoObjects` |

Every test and every step is written the same way, one field on each line, in the order `Name`, `Setup` or `Change`, `Run`, `Expect`, `Ignore`:

```go
suite.Test{
	Name: "LeadershipLostAndRegained",
	Steps: []suite.Step{
		{
			Name:   "first run",
			Expect: suite.Live(suite.Collected),
		},
		{
			Name:   "lose the leadership",
			Change: suite.SetLeader(false),
			Expect: suite.NoObjects,
		},
		...
	},
}
```

A special case uses a closure for `Change` or `Expect`, and builds its payloads with `env.PayloadsOf` and `Payloads.With`.

## Expected payloads

The expected payloads of an object are built from its current state, with the rules the check applies to its kind: the payload type, the tags, the removed fields and the scrubbed values.
Values that a test cannot predict, such as the pod version hash and the deletion time, are replaced on both sides.
A difference names the field, as the manifest content is decoded before the comparison.

## Resources and fixtures

`suite/resources.go` lists the resources of the check, and `suite/kubernetes_fixtures.go` holds an object of each.
When it creates an Env, the suite fails unless the list matches the collectors of the check and every resource has a fixture, or a reason in `suite.NotListed`.
So every test covers every resource: a new collector fails every test until it is listed and has a fixture, and then every test includes it.

The payload comparisons check the rest of the list: a wrong kind or payload type fails the first run, wrong deletion support fails the deletion steps, and a wrong default selection fails `Configurations/DefaultCollectors`.

## Add a test

1. Choose the file of its area in `cases/`, or add one that registers a group in `init`.
2. Add a `suite.Test` with its Setup, if it is not the default, and its Steps.
3. Use the step vocabulary, and a closure only for what is special about the test.

## Run

```bash
dda inv test --targets=./pkg/collector/corechecks/cluster/orchestrator/checktest
```

```bash
bazel test //pkg/collector/corechecks/cluster/orchestrator/checktest:checktest_test_dca
```
