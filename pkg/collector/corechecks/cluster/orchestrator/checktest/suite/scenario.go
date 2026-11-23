// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	orchestratorcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/registry"
	"github.com/DataDog/datadog-agent/pkg/config/env"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// assertNoSensitiveData checks that no payload carries a field that the check must remove: the fixtures put "secret"
// in the last-applied-configuration annotation and in a container environment variable, and set managed fields, which
// the manifest-only kinds remove.
func assertNoSensitiveData(t *testing.T, expected ExpectedSet, sent []SentResource) {
	t.Helper()
	for _, s := range sent {
		assert.False(t, bytes.Contains(s.Content, []byte(`"secret"`)), "%s %s sends a sensitive value: %s", s.Envelope, s.UID, s.Content)
		if e := expected.Live[s.UID]; slices.Contains([]string{"v1/configmaps", "v1/secrets", "v1/helm-releases-configmaps"}, e.FullName) {
			assert.False(t, bytes.Contains(s.Content, []byte("managedFields")), "%s %s sends managed fields", s.Envelope, s.UID)
		}
	}
}

// The harness runs the real orchestrator check against a fake API server that serves a fixture of every registry
// entry, and returns what the check passes to a mock sender.

const scenarioClusterID = "d801b2b1-4811-11ea-8618-121d4d0938a3"

// DeploymentMode is where the check runs.
type DeploymentMode string

const (
	ClusterAgentLeader   DeploymentMode = "cluster-agent-leader"
	ClusterAgentFollower DeploymentMode = "cluster-agent-follower"
	ClusterCheckRunner   DeploymentMode = "cluster-check-runner"
)

// ScenarioConfig is the check configuration of a scenario. Manifest collection, the manifest buffer and terminated
// resources are always enabled, as in their defaults.
type ScenarioConfig struct {
	DedicatedPipeline bool
	// AllCollectors selects every built-in collector in the check configuration instead of the default selection.
	AllCollectors bool
	// OlderVersions serves the last version of each registry entry that has several, instead of the first one.
	OlderVersions bool
	// Instance replaces the collector selection of the check instance configuration.
	Instance string
	// Undiscovered are the full names of the fixtures that API discovery does not report, until the test adds them.
	Undiscovered []string
	// Setup changes the dependencies of the check before it is configured.
	Setup func(t *testing.T, m *ScenarioMocks)
}

func (c ScenarioConfig) String() string {
	return fmt.Sprintf("pipeline=%t,all=%t,older=%t", c.DedicatedPipeline, c.AllCollectors, c.OlderVersions)
}

// ScenarioMocks are the dependencies of the check in a scenario. A case changes them in ScenarioConfig.Setup, before
// the check is configured, or between runs.
type ScenarioMocks struct {
	// Config is the Agent configuration, with the settings of NewScenario.
	Config configmodel.BuildableConfig
	// Cluster is the fake API server. It serves a fixture of every registry entry.
	Cluster *registry.TestCluster
	// Discovery is the API discovery of the fake API server.
	Discovery *FakeDiscovery
	// Leader is the result of the leader election.
	Leader       bool
	Tagger       taggermock.Mock
	Workloadmeta workloadmetamock.Mock
	// Sink records what the check sends.
	Sink *sendRecorder
}

// FakeDiscovery serves the API discovery of the fake cluster, or the error that a case sets.
type FakeDiscovery struct {
	Cluster *registry.TestCluster
	mu      sync.Mutex
	err     error
}

// Fail makes the discovery return the error, or the discovery of the fake cluster again when it is nil.
func (d *FakeDiscovery) Fail(err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.err = err
}

func (d *FakeDiscovery) serverGroupsAndResources() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
	d.mu.Lock()
	err := d.err
	d.mu.Unlock()
	if err != nil {
		return nil, nil, err
	}
	return d.Cluster.ServerGroupsAndResources()
}

// Scenario is a configured check that runs against a fake cluster of the registry fixtures.
type Scenario struct {
	Mode     DeploymentMode
	Cfg      ScenarioConfig
	Chk      *orchestratorcheck.OrchestratorCheck
	Mocks    *ScenarioMocks
	Objects  []ScenarioObject
	Expected ExpectedSet
	// Run runs the check and returns the resources sent since the last run.
	Run func() []SentResource
	// Cancel cancels the check once, and returns the resources sent since the last run.
	Cancel func() []SentResource
}

func NewScenario(t *testing.T, senderManager sender.SenderManager, mode DeploymentMode, cfg ScenarioConfig) *Scenario {
	resetResourceCaches(t)
	config := mockconfig.New(t)
	env.SetFeatures(t, env.Kubernetes)
	clustername.ResetClusterName()
	clustername.DisableNodeLabelLookupForTest(t)
	t.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", scenarioClusterID)
	for key, value := range map[string]any{
		"cluster_name": "test-cluster",
		// A configured hostname and no cloud provider avoid metadata endpoint timeouts.
		"hostname":                      "test-host",
		"cloud_provider_metadata":       []string{},
		"leader_election":               true,
		"orchestrator_explorer.enabled": true,
		"orchestrator_explorer.manifest_collection.enabled":                    true,
		"orchestrator_explorer.manifest_collection.buffer_manifest":            true,
		"orchestrator_explorer.manifest_collection.dedicated_pipeline.enabled": cfg.DedicatedPipeline,
		"orchestrator_explorer.terminated_resources.enabled":                   true,
		"orchestrator_explorer.terminated_pods.enabled":                        true,
		"orchestrator_explorer.helm_releases.configmaps.enabled":               true,
		// Every built-in custom resource family is enabled, so that each registry entry is collected.
		"orchestrator_explorer.custom_resources.ootb.enabled":             true,
		"orchestrator_explorer.custom_resources.ootb.gateway_api":         true,
		"orchestrator_explorer.custom_resources.ootb.service_mesh":        true,
		"orchestrator_explorer.custom_resources.ootb.ingress_controllers": true,
	} {
		config.SetInTest(key, value)
	}

	objects := ScenarioObjects(t, cfg)
	runtimeObjects := make([]runtime.Object, 0, len(objects))
	for _, o := range objects {
		runtimeObjects = append(runtimeObjects, o.Object)
	}
	cluster := registry.NewTestCluster(t, runtimeObjects)
	cluster.Resources = CustomResourceDiscovery(objects, cfg)
	cluster.Versions = func(resource registry.Resource) []string { return []string{ServedVersion(resource, cfg)} }

	mocks := &ScenarioMocks{
		Config:       config,
		Cluster:      cluster,
		Discovery:    &FakeDiscovery{Cluster: cluster},
		Leader:       mode != ClusterAgentFollower,
		Tagger:       taggerfxmock.SetupFakeTagger(t),
		Workloadmeta: fxutil.Test[workloadmetamock.Mock](t, fx.Options(core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))),
	}
	if cfg.Setup != nil {
		cfg.Setup(t, mocks)
	}
	chk := newScenarioCheck(t, senderManager, mode, cfg, mocks)
	var cancelOnce sync.Once
	cancel := func() []SentResource {
		cancelOnce.Do(func() { runWithTimeout(t, chk.Cancel) })
		return mocks.Sink.take(t)
	}
	t.Cleanup(func() { cancel() })

	return &Scenario{
		Mode:     mode,
		Cfg:      cfg,
		Chk:      chk,
		Mocks:    mocks,
		Objects:  objects,
		Expected: ExpectedObjects(objects, cfg),
		Run: func() []SentResource {
			runWithTimeout(t, func() { require.NoError(t, chk.Run()) })
			return mocks.Sink.take(t)
		},
		Cancel: cancel,
	}
}

// Object returns the fixture of a collector.
func (sc *Scenario) Object(t *testing.T, fullName string) ScenarioObject {
	for _, o := range sc.Objects {
		if o.FullName == fullName {
			return o
		}
	}
	require.Failf(t, "no fixture", "no fixture for %s", fullName)
	return ScenarioObject{}
}

// Update replaces an object of the cluster, and its state in the scenario.
func (sc *Scenario) Update(t *testing.T, updated runtime.Object) {
	sc.Mocks.Cluster.Update(t, updated)
	sc.setState(updated)
}

// Delete deletes an object of the cluster, and keeps its last state in the scenario.
func (sc *Scenario) Delete(t *testing.T, object runtime.Object) runtime.Object {
	last := sc.Mocks.Cluster.Delete(t, object)
	sc.setState(last)
	return last
}

func (sc *Scenario) setState(object runtime.Object) {
	uid := ScenarioObject{Object: object}.UID()
	for i, o := range sc.Objects {
		if o.UID() == uid {
			sc.Objects[i].Object = object
		}
	}
	if e, found := sc.Expected.Live[uid]; found {
		e.Object = object
		sc.Expected.Live[uid] = e
	}
}

// Collectors returns the collectors of every bundle with the full name.
func (sc *Scenario) Collectors(fullName string) []collectors.K8sCollector {
	return sc.Chk.CollectorsForTest(fullName)
}

// Collected returns the objects of the scenario that the check collects.
func (sc *Scenario) Collected() []ScenarioObject {
	var collected []ScenarioObject
	for _, o := range sc.Objects {
		if _, found := sc.Expected.Live[o.UID()]; found {
			collected = append(collected, o)
		}
	}
	return collected
}

// DeleteAll deletes the objects, and waits until no informer holds them.
func (sc *Scenario) DeleteAll(t *testing.T, objects []ScenarioObject) {
	for _, o := range objects {
		sc.Delete(t, o.Object)
	}
	sc.WaitGone(t, objects)
}

// WaitGone waits until no informer holds the objects, and lets their delete handlers run. TestCluster.Delete updates
// an object before it deletes it, so a run between the two events would correctly send the update as live.
func (sc *Scenario) WaitGone(t *testing.T, objects []ScenarioObject) {
	t.Helper()
	require.Eventually(t, func() bool {
		for _, o := range objects {
			for _, collector := range sc.Collectors(o.FullName) {
				for _, object := range collector.Informer().GetStore().List() {
					if accessor, err := meta.Accessor(object); err == nil && string(accessor.GetUID()) == o.UID() {
						return false
					}
				}
			}
		}
		return true
	}, 5*time.Second, 5*time.Millisecond, "the informers still hold deleted objects")
	time.Sleep(100 * time.Millisecond)
}

// newScenarioCheck returns a check configured with the mocks, and sets the sink of the mocks.
func newScenarioCheck(t *testing.T, senderManager sender.SenderManager, mode DeploymentMode, cfg ScenarioConfig, mocks *ScenarioMocks) *orchestratorcheck.OrchestratorCheck {
	quietLogs(t)
	chk := orchestratorcheck.NewCheckForTest(mocks.Config, mocks.Workloadmeta, mocks.Tagger, orchestratorcheck.TestDependencies{
		WaitForAPIClient: func(context.Context) (*apiserver.APIClient, error) { return mocks.Cluster.APIClient, nil },
		RunLeaderElection: func() (string, error) {
			if mode == ClusterCheckRunner {
				t.Error("a cluster check runner that skips the leader election must not run it")
			}
			if mocks.Leader {
				return "self", nil
			}
			return "other", apiserver.ErrNotLeader
		},
		ServerGroupsAndResources: mocks.Discovery.serverGroupsAndResources,
		IsCLCRunner:              mode == ClusterCheckRunner,
	})

	recorder := newSendRecorder(chk, senderManager)
	var instance strings.Builder
	if mode == ClusterCheckRunner {
		instance.WriteString("skip_leader_election: true\n")
	}
	instance.WriteString(cfg.Instance)
	if cfg.AllCollectors {
		instance.WriteString("collectors:\n")
		for _, resource := range registry.Resources() {
			if !resource.IsCustomResource() {
				instance.WriteString("  - " + resource.Resource + "\n")
			}
		}
		instance.WriteString("  - clusters\n")
	}
	require.NoError(t, chk.Configure(senderManager, 1, integration.Data(instance.String()), integration.Data{}, "test", "provider"))
	// Configure derives the check ID from its configuration.
	mocksender.SetSender(recorder.sender, chk.ID())
	mocks.Sink = recorder
	return chk
}

// resetResourceCaches empties the resource caches, and restores them when the test ends.
func resetResourceCaches(t *testing.T) {
	resourceCache, manifestCache := orchestrator.KubernetesResourceCache, orchestrator.KubernetesManifestCache
	t.Cleanup(func() {
		orchestrator.KubernetesResourceCache, orchestrator.KubernetesManifestCache = resourceCache, manifestCache
	})
	orchestrator.KubernetesResourceCache = orchestrator.NewKubernetesResourceCache()
	orchestrator.KubernetesManifestCache = orchestrator.NewKubernetesResourceCache()
}

// runWithTimeout fails the test when the function blocks, as a blocked check run would otherwise hang the suite.
func runWithTimeout(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the check is blocked")
	}
}

// quietLogs replaces the trace logger that the component mocks install with a warning one, as the scenarios run the
// check many times and debug logs slow them down.
func quietLogs(t *testing.T) {
	logger, err := pkglog.LoggerFromWriterWithMinLevelAndFullFormat(testLogWriter{t}, pkglog.WarnLvl)
	require.NoError(t, err)
	pkglog.SetupLogger(logger, pkglog.WarnStr)
	t.Cleanup(logger.Close)
}

type testLogWriter struct{ t *testing.T }

func (w testLogWriter) Write(p []byte) (int, error) {
	w.t.Log(strings.TrimSuffix(string(p), "\n"))
	return len(p), nil
}

// eventually runs the check until the condition holds on everything sent, as informers report changes
// asynchronously, and returns everything sent.
func eventually(t *testing.T, run func() []SentResource, condition func([]SentResource) bool) []SentResource {
	t.Helper()
	var sent []SentResource
	deadline := time.Now().Add(5 * time.Second)
	for {
		sent = append(sent, run()...)
		if condition(sent) || time.Now().After(deadline) {
			// Let late events arrive, so that duplicates show up.
			time.Sleep(30 * time.Millisecond)
			return append(sent, run()...)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// WaitQueued waits until the collectors queued the deletions of the objects whose deletion the check sends. The
// terminated pods collector queues the deletions of pods.
func (sc *Scenario) WaitQueued(t *testing.T, objects []ScenarioObject) {
	t.Helper()
	queued := map[string]int{}
	for _, o := range objects {
		switch e := sc.Expected.Live[o.UID()]; {
		case e.FullName == "v1/pods":
			queued["v1/terminated-pods"]++
		case e.Deletes:
			queued[o.FullName]++
		}
	}
	require.Eventually(t, func() bool {
		for fullName, count := range queued {
			for _, collector := range sc.Collectors(fullName) {
				if queue, ok := collector.(interface{ QueuedTerminated() int }); !ok || queue.QueuedTerminated() < count {
					return false
				}
			}
		}
		return true
	}, 5*time.Second, 10*time.Millisecond, "the collectors did not queue the deletions")
}

// AddCopies adds a copy of each object to the cluster, with its own name and UID, and returns the copies.
func (sc *Scenario) AddCopies(t *testing.T, objects []ScenarioObject) []ScenarioObject {
	var copies []ScenarioObject
	for _, o := range objects {
		c := o
		c.Object = o.Object.DeepCopyObject()
		accessor, err := meta.Accessor(c.Object)
		require.NoError(t, err)
		accessor.SetUID(accessor.GetUID() + "-copy")
		accessor.SetName(accessor.GetName() + "-copy")
		accessor.SetResourceVersion("1")
		sc.Mocks.Cluster.Add(t, c.Object)
		sc.Objects = append(sc.Objects, c)
		if e, found := sc.Expected.Live[o.UID()]; found {
			e.Object = c.Object
			sc.Expected.Live[c.UID()] = e
		}
		copies = append(copies, c)
	}
	return copies
}
