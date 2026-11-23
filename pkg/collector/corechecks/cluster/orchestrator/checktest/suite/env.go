// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package suite

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
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
	configenv "github.com/DataDog/datadog-agent/pkg/config/env"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const clusterID = "d801b2b1-4811-11ea-8618-121d4d0938a3"

// Env is the environment of a test in one setup: the real orchestrator check, configured against a fake Kubernetes
// API server that serves an object of every resource, with the mocks of its other dependencies. A step changes the
// environment through its methods.
type Env struct {
	Setup Setup
	Check *orchestratorcheck.OrchestratorCheck
	// Cluster is the fake API server.
	Cluster *TestCluster
	// Objects are the objects of the fake cluster, in their current state.
	Objects []*Object
	// Config is the Agent configuration.
	Config       configmodel.BuildableConfig
	Tagger       taggermock.Mock
	Workloadmeta workloadmetamock.Mock

	discovery *fakeDiscovery
	leader    bool
	// deleted are the last states of the objects that Delete deleted, by UID.
	deleted map[string]runtime.Object
	// nodesChanged reports that nodes were added or deleted since the check last sent the cluster, which combines them.
	nodesChanged bool
	sink         *sendRecorder
	run          func() []Sent
}

// NewEnv returns a configured check, and the fake cluster and mocks that it runs against.
func NewEnv(t *testing.T, senderManager sender.SenderManager, setup Setup) *Env {
	resetResourceCaches(t)
	config := mockconfig.New(t)
	configenv.SetFeatures(t, configenv.Kubernetes)
	clustername.ResetClusterName()
	clustername.DisableNodeLabelLookupForTest(t)
	t.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", clusterID)
	for key, value := range map[string]any{
		"cluster_name": "test-cluster",
		// A configured hostname and no cloud provider avoid metadata endpoint timeouts.
		"hostname":                      "test-host",
		"cloud_provider_metadata":       []string{},
		"leader_election":               true,
		"orchestrator_explorer.enabled": true,
		"orchestrator_explorer.manifest_collection.enabled":         true,
		"orchestrator_explorer.manifest_collection.buffer_manifest": true,
		"orchestrator_explorer.terminated_resources.enabled":        true,
		"orchestrator_explorer.terminated_pods.enabled":             true,
		"orchestrator_explorer.helm_releases.configmaps.enabled":    true,
		// Every built-in custom resource family is enabled, so that each resource is collected.
		"orchestrator_explorer.custom_resources.ootb.enabled":             true,
		"orchestrator_explorer.custom_resources.ootb.gateway_api":         true,
		"orchestrator_explorer.custom_resources.ootb.service_mesh":        true,
		"orchestrator_explorer.custom_resources.ootb.ingress_controllers": true,
	} {
		config.SetInTest(key, value)
	}

	objects := newObjects(t, setup)
	runtimeObjects := make([]runtime.Object, 0, len(objects))
	for _, o := range objects {
		runtimeObjects = append(runtimeObjects, o.State)
	}
	cluster := newTestCluster(t, runtimeObjects)
	cluster.Resources = customResourceDiscovery(objects, setup)
	cluster.Versions = func(resource Resource) []string { return []string{ServedVersion(resource, setup)} }

	env := &Env{
		Setup:        setup,
		Cluster:      cluster,
		Objects:      objects,
		Config:       config,
		Tagger:       taggerfxmock.SetupFakeTagger(t),
		Workloadmeta: fxutil.Test[workloadmetamock.Mock](t, fx.Options(core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))),
		discovery:    &fakeDiscovery{cluster: cluster},
		leader:       setup.Mode != ClusterAgentFollower,
		deleted:      map[string]runtime.Object{},
		nodesChanged: true,
	}
	if setup.BeforeConfigure != nil {
		setup.BeforeConfigure(t, env)
	}
	requireResourcesMatchCheck(t, env)
	env.configureCheck(t, senderManager)

	t.Cleanup(func() { _ = runWithTimeout(t, func() error { env.Check.Cancel(); return nil }) })
	env.run = func() []Sent {
		require.NoError(t, runWithTimeout(t, env.Check.Run))
		return env.sink.take()
	}
	return env
}

// ServeUndiscovered makes API discovery report the custom resources of Setup.Undiscovered, which the check then
// collects.
func (env *Env) ServeUndiscovered() {
	env.Setup.Undiscovered = nil
	for _, o := range env.Objects {
		o.Collected = isCollected(o, env.Setup)
	}
	env.Cluster.Resources = customResourceDiscovery(env.Objects, env.Setup)
}

// Collected returns the objects that the check collects.
func (env *Env) Collected() []*Object {
	var collected []*Object
	for _, o := range env.Objects {
		if o.Collected {
			collected = append(collected, o)
		}
	}
	return collected
}

// update replaces an object of the cluster, and its state.
func (env *Env) update(t *testing.T, updated runtime.Object) {
	env.Cluster.Update(t, updated)
	env.setState(updated)
}

func (env *Env) setState(state runtime.Object) {
	if o := env.objectByUID(uidOf(state)); o != nil {
		o.State = state
	}
}

func (env *Env) objectByUID(uid string) *Object {
	for _, o := range env.Objects {
		if o.uid() == uid {
			return o
		}
	}
	return nil
}

// addCopies adds a copy of each object to the cluster, with its own name and UID.
func (env *Env) addCopies(t *testing.T, objects []*Object) {
	for _, o := range objects {
		c := *o
		c.Copy = true
		c.State = o.State.DeepCopyObject()
		accessor, err := meta.Accessor(c.State)
		require.NoError(t, err)
		accessor.SetUID(accessor.GetUID() + "-copy")
		accessor.SetName(accessor.GetName() + "-copy")
		accessor.SetResourceVersion("1")
		env.Cluster.Add(t, c.State)
		env.Objects = append(env.Objects, &c)
		env.nodesChanged = env.nodesChanged || isNode(c.State)
	}
}

// deleteAll deletes the objects, and waits until the check has queued the deletions that it reports.
func (env *Env) deleteAll(t *testing.T, objects []*Object) {
	queued, _ := env.Check.PendingDeletionsForTest()
	want := queued
	for _, o := range objects {
		if o.Collected && env.reportsDeletion(o) {
			want++
		}
		o.State = env.Cluster.Delete(t, o.State)
		env.nodesChanged = env.nodesChanged || isNode(o.State)
	}
	env.waitDeleted(t, objects, want)
}

// Delete deletes an object that is not one of Objects and whose deletion the check reports, and waits until the check
// has queued it.
func (env *Env) Delete(t *testing.T, object runtime.Object) {
	queued, _ := env.Check.PendingDeletionsForTest()
	env.deleted[uidOf(object)] = env.Cluster.Delete(t, object)
	env.waitDeleted(t, nil, queued+1)
}

// Deleted returns the last state of an object that Delete deleted.
func (env *Env) Deleted(t *testing.T, uid string) runtime.Object {
	last, found := env.deleted[uid]
	require.True(t, found, "%s was not deleted", uid)
	return last
}

func isNode(object runtime.Object) bool {
	_, ok := object.(*corev1.Node)
	return ok
}

// reportsDeletion reports whether the check sends the deletion of an object. Pods are reported by the terminated pods
// collector, as the pods collector ignores deletions.
func (env *Env) reportsDeletion(o *Object) bool {
	if o.FullName == "v1/nodes" {
		// Known defect: a shared informer only gets the delete handler of the collector that starts it first. When the
		// cluster collector, which collects no deletions, starts the node informer, deleted nodes are not reported.
		order := env.Check.CollectorOrderForTest()
		if clusters := slices.Index(order, "clusters"); clusters >= 0 && clusters < slices.Index(order, "v1/nodes") {
			return false
		}
	}
	return !o.Resource.IgnoreDeletions || o.FullName == "v1/pods"
}

// waitDeleted waits until no informer holds the objects and, when the check queues deletions, until it queued want
// deletions. A run before that could send the update that TestCluster.Delete makes first.
func (env *Env) waitDeleted(t *testing.T, objects []*Object, want int) {
	t.Helper()
	if env.Setup.Mode == ClusterAgentFollower {
		// A follower that never led has started no informer, and queues no deletion.
		return
	}
	require.Eventually(t, func() bool {
		for _, o := range objects {
			for _, collector := range env.Check.CollectorsForTest(o.FullName) {
				for _, object := range collector.Informer().GetStore().List() {
					if uidOf(object) == o.uid() {
						return false
					}
				}
			}
		}
		return true
	}, 5*time.Second, 5*time.Millisecond, "the informers still hold deleted objects")
	if _, enabled := env.Check.PendingDeletionsForTest(); !enabled {
		// A follower drops the deletions: give the delete handlers time to run before the leadership can change.
		time.Sleep(100 * time.Millisecond)
		return
	}
	require.Eventually(t, func() bool {
		queued, _ := env.Check.PendingDeletionsForTest()
		return queued >= want
	}, 5*time.Second, 5*time.Millisecond, "the check did not queue the deletions")
}

// configureCheck creates and configures the check of the environment, with a sender that records what it sends.
func (env *Env) configureCheck(t *testing.T, senderManager sender.SenderManager) {
	quietLogs(t)
	mode := env.Setup.Mode
	env.Check = orchestratorcheck.NewCheckForTest(env.Config, env.Workloadmeta, env.Tagger, orchestratorcheck.TestDependencies{
		WaitForAPIClient: func(context.Context) (*apiserver.APIClient, error) { return env.Cluster.APIClient, nil },
		RunLeaderElection: func() (string, error) {
			if mode == ClusterCheckRunner {
				t.Error("a cluster check runner that skips the leader election must not run it")
			}
			if env.leader {
				return "self", nil
			}
			return "other", apiserver.ErrNotLeader
		},
		ServerGroupsAndResources: env.discovery.serverGroupsAndResources,
		IsCLCRunner:              mode == ClusterCheckRunner,
	})

	recorder := newSendRecorder(env.Check, senderManager)
	var instance strings.Builder
	if mode == ClusterCheckRunner {
		instance.WriteString("skip_leader_election: true\n")
	}
	instance.WriteString(env.Setup.Instance)
	if !env.Setup.DefaultCollectors && env.Setup.Instance == "" {
		instance.WriteString("collectors:\n")
		for _, resource := range Resources() {
			if !resource.IsCustomResource() {
				instance.WriteString("  - " + resource.Resource + "\n")
			}
		}
		instance.WriteString("  - clusters\n")
	}
	require.NoError(t, env.Check.Configure(senderManager, 1, integration.Data(instance.String()), integration.Data{}, "test", "provider"))
	// Configure derives the check ID from its configuration.
	mocksender.SetSender(recorder.sender, env.Check.ID())
	env.sink = recorder
}

// resetResourceCaches empties the resource cache, and restores it when the test ends.
func resetResourceCaches(t *testing.T) {
	resourceCache := orchestrator.KubernetesResourceCache
	t.Cleanup(func() { orchestrator.KubernetesResourceCache = resourceCache })
	orchestrator.KubernetesResourceCache = orchestrator.NewKubernetesResourceCache()
}

// quietLogs replaces the trace logger that the component mocks install with a warning one, as the tests run the
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
