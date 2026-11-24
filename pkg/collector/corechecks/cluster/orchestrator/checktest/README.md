# Orchestrator check tests

`checktest` tests the Cluster Agent orchestrator check as a whole.
The real check runs against a fake Kubernetes cluster, and each test checks what the check passes to its sender.
The tests do not cover the network, the intake or the backend.
The processor and transformer tests check how each field of a model is built.

## Layout

```
checktest/
├── *_test.go             the tests, one file for each area: every resource, leadership, lifecycle, cluster check
│                         runners, discovery and CRDs, collector selection and dependencies, settings and Helm
│                         releases, tags, pods, failures
├── suite/                the environment of the tests
│   ├── env.go            Setup, its options, the actions, and Pop
│   ├── expected.go       the assertions, and the payload files
│   ├── cluster.go        the fake API server and API discovery
│   ├── fixtures.go       the default objects of the cluster
│   ├── objects.go        small objects for the tests that replace the default ones
│   └── sent.go           what the check sent, and how payloads are normalized
└── testdata/
    ├── manifests/        input: a Kubernetes manifest for each collector of a built-in resource
    └── payloads/         expected: what the check sends for each object, with the name of its manifest
```

`testdata/manifests/apps_v1_deployments.yaml` is the Deployment of the cluster, and `testdata/payloads/apps_v1_deployments.yaml` is what the check sends for it: its manifest and metadata payloads under `live`, and under `deleted` for the objects whose deletion a test checks.
The UID of each object is the name of its files, so a test expects `live apps_v1_deployments`.

## The pattern of a test

Every test follows the same steps:

```go
func TestLeadership(t *testing.T) {
	t.Run("deletions_while_following_are_dropped", func(t *testing.T) {
		// 1. Set up the environment, with the options that the test needs.
		a, b := suite.Deployment("a"), suite.Deployment("b")
		env := suite.Setup(t, suite.WithCollectors("deployments"), suite.WithObjects(a, b))

		// 2. Act on the cluster or the check.
		env.SetLeader(false)
		...
		env.Delete(t, b)

		// 3. Run the check, take what it sent, and compare it with what the test expects.
		suite.AssertObjects(t, []string{"deleted b"}, env.Pop(t, "deleted b"))
	})
}
```

To add a test, add a `t.Run` to the test of its area, or a new `TestX` in a new file, with these steps.
Start from the default cluster, with an object of every resource, and expect every object except the ones that the test is about, so that a bug in any collector shows: a bug can be in the informer, the processor or the cache handling of one resource only.
`suite.Live(env.UIDs()...)` and the helpers of `helpers_test.go` (`firstRun`, `deletion`, `except`, `withPrefix`) build these expectations.
Use smaller objects only where the subject is one resource, like the scheduling of a pod or the version of one custom resource, and say why.

### 1. Set up

`suite.Setup(t)` returns the default environment: a fake cluster with an object of every resource that the check collects, and the check as the Cluster Agent leader with every built-in collector.
Options customize it:

| Option | Effect |
|---|---|
| `WithObjects(objects...)` | the cluster starts with these objects instead: `suite.Deployment`, `suite.Service`, `suite.Node`, `suite.PendingPod`, `suite.AssignedPod`, any Kubernetes object, `suite.CustomResource(t, "argoproj.io/v1alpha1/rollouts")` for a built-in custom resource, or `suite.CronTab(name)` for one that the check does not know; `suite.Named(t, object, name)` renames a copy |
| `WithMode` | `ClusterAgentLeader` (default), `ClusterAgentFollower` or `ClusterCheckRunner` |
| `WithCollectors(names...)` | the collectors of the instance configuration |
| `WithInstance(yaml)` | the whole instance configuration; `""` leaves the collector selection to the check |
| `WithCRDCollectorsOnly()` | a CRD collector for each built-in custom resource, and no other collector |
| `WithSetting(key, value)` | an Agent setting, like `leader_election` |
| `WithOlderVersions()` | the cluster serves the last version of each resource that has several |
| `WithDiscoveryFailure(err)` | API discovery fails from the start |
| `WithFailedGroupVersions(gvs...)` | API discovery fails for these group versions only |
| `WithUndiscoveredCustomResources()` | discovery reports the custom resources only after `env.InstallCRDs` or `env.Discover` |
| `WithExtraObjects(objects...)` | more objects in the cluster before the check is configured, with the default ones or those of `WithObjects` |
| `WithoutCollectors(names...)` | the default selection without these collectors |
| `WithExtraSyncTimeout(seconds)` | `extra_sync_timeout_seconds` of the instance configuration |
| `WithInstanceTags(tags...)`, `WithInitConfig(yaml)` | the tags of the check configuration, like a dispatched cluster check |
| `WithGlobalTags(tags...)` | the global tags that the tagger reports, like `orchestrator_explorer.extra_tags`; `WithGlobalTagsByCardinality` sets each cardinality |
| `WithoutSkipLeaderElection()` | a cluster check runner configuration without `skip_leader_election` |
| `WithForbiddenResources(collectors...)` | RBAC forbids listing these custom resources |
| `WithFailingLists(collectors...)` | every list of these built-in resources fails, so their informers never sync |

Select only the collectors that a test needs, so that it expects only the objects that it is about.
Two rules of the check matter here: the cluster collector needs a `kube-system` namespace, and the check collects the built-in custom resources only when the CRD collector is selected.

### 2. Act

`env.Add`, `env.Update`, `env.Delete`, `env.DeleteUIDs`, `env.UpdateAll`, `env.DeleteAll` and `env.AddCopies` change the cluster, and `env.Object(t, uid)`, `env.UIDs()` and `env.CustomResourceUIDs()` read it.
`suite.Updated(t, object)` returns a copy of an object with a new label and the next resource version.
`env.SetLeader`, `env.FailLeaderElection`, `env.FailDiscovery`, `env.FailGroupVersions`, `env.Discover`, `env.InstallCRDs` and `env.SetInfraTags` change what the check sees; `suite.CRD(t, object, established)` returns the CRD of a custom resource.
`env.Stop` cancels the check and returns what it flushed, `env.Restart` replaces it with a new check in the same process, and `env.ExpireResourceCache` expires the cache of the resource versions sent, as three minutes do.
`env.SetEntityTags(tags...)` makes the tagger report tags for every object, as the processors look them up.
`env.AddCheck(t, options...)` configures another instance of the check in the same process, against the same cluster, like a cluster check runner with several instances; change the cluster through the first environment.
`env.Config` and `env.Tagger` give access to the rest, and `env.LeaderElections()` counts the elections.

The check learns that it lost the leadership when it runs: call `env.Pop` after `env.SetLeader(false)` and before the changes that it must not see.

### 3. Pop and assert

`env.Pop(t, objects...)` runs the check until it has sent each object, then returns everything that it sent.
An object is `live <uid>` or `deleted <uid>`, and the cluster is `live cluster`.
`env.Pop(t)` with no object expects nothing. No test waits for a fixed time: after a change that nothing sent can show, a test waits for a signal first, like `env.WaitForDeletions(t, objects...)`, which the check reports through its `OnDeletion` test dependency, or the update of another object of the same informer, which handles its events in order.
`env.Pop` fails the test when a run fails; `env.PopWithError(t)` returns the error instead.
`actual.ManifestContent(t, "live a")` returns the decoded manifest of an object, for a test about a few fields.

Then compare the result:

| Assertion | Checks |
|---|---|
| `suite.AssertObjects(t, expected, actual)` | which objects were sent, and that no payload was sent twice; `suite.Live(uids...)` and `suite.Deleted(uids...)` build the list |
| `suite.AssertPayloadFiles(t, actual, "live a")` | the full content of the payloads of the objects, with their payload files |
| `suite.AssertPayloads(t, expected, actual)` | the full content, with payloads that the test builds: `suite.ReadPayloads(t, objects...)` reads the payload files, `Payloads.WithMessageTags` adds tags, and `Payloads.Walk` changes values, like an environment variable |
| `suite.AssertUpdated(t, actual)` | every payload has the label that `env.UpdateAll` adds |
| `suite.AssertDeleted(t, actual)` | every payload reports a deleted object, with a deletion timestamp |

Check which objects are sent at every step: a payload check cannot notice a payload that is missing.
Check the full content once for each resource, in `TestEveryResource`.
A test about a variation builds its expected payloads from the payload files and states the difference, so the difference is visible in the test:

```go
env := suite.Setup(t, suite.WithMode(suite.ClusterCheckRunner))
env.SetInfraTags("infra_mode:cloud_cost_only")

expected := suite.ReadPayloads(t, contentObjects(t)...).WithMessageTags("infra_mode:cloud_cost_only")
actual := env.Pop(t, expected.Objects()...)
suite.AssertPayloads(t, expected, actual)
```

## TestEveryResource

It runs the default environment through three steps (first run, update every object, delete every object), in four configurations: leader, cluster check runner, default collectors and older versions.

- Each step expects every object of the cluster, except the `notCollected` objects of the configuration, each with its reason in a comment, and the cluster when the nodes change. The known defect that the check does not report deleted nodes is stated at the delete step.
- The leader and the cluster check runner compare the first-run content of every built-in resource, of the cluster, and of two custom resources: the built-in custom resources share their collector and processor, so a resource-specific one and a group-wide one cover them. They also compare the deleted pod, whose status and version change.
- The update and delete steps check the generic fields of every payload with `AssertUpdated` and `AssertDeleted`.

## The fake cluster

The fake clients of client-go differ from an API server. The suite makes them behave like one where the check depends on it:

- A watch applies its label and field selectors. It reports an object that starts matching them as added, and one that stops matching them as deleted, like a pod that a node runs for the unassigned pod informer.
- A deleted object has a new resource version, as the check skips a resource version that it already sent.
- The metadata client also serves the ConfigMaps and Secrets of the typed client.
- API discovery reports every built-in resource, the custom resources of the cluster, and the failures that a test sets.

## Payload files

A payload file lists the payloads of an object as JSON values: the manifest content is decoded, so that a difference names the field.
The suite replaces the values that a test cannot predict: the deletion time, the version of a pod (a hash of its model, which the payload holds, with the deletion time), and the managed fields, which the fake clients generate and the files show as `<managed fields>`.
Lists that the check builds from maps (tags, labels, annotations) are sorted.

Each run also checks that no payload, manifest or metadata model, holds a sensitive value: the fixtures put `secret` in the last-applied-configuration annotation and in container environment variables, and the manifests of ConfigMaps and Secrets must not include managed fields.
`Setup` restores the global list of sensitive annotations and labels when a test ends, as its setting only adds to it.

To change what the check sends, run the tests with `-update`, and review the diff of `testdata/payloads`.
`-update` writes the payloads that the tests compare, but does not delete the files or sections that they no longer compare: delete those with the change that stops comparing them.

## Add a collector

1. Add a manifest to `testdata/manifests/<group>_<version>_<resource>.yaml` (the collector full name, with underscores for slashes), with that name as its UID. `TestFixtures` fails until each collector has one.
   Built-in custom resources need no fixture: the suite generates one for each entry of the check.
2. Run the tests with `-update`, and review the new file in `testdata/payloads`.

## Run

```bash
dda inv test --targets=./pkg/collector/corechecks/cluster/orchestrator/checktest
```

Update the expected files from the source tree, as Bazel runs the tests in a sandbox:

```bash
go test -tags kubeapiserver,test ./pkg/collector/corechecks/cluster/orchestrator/checktest -update
```

The suite reaches the check only through `../testing.go`: its API client, leader election and API discovery, a callback after it handles a deletion, the collector inventory and the list of built-in custom resources.
