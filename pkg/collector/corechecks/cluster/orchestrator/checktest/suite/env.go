// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

// Package suite runs the real orchestrator check against a fake Kubernetes cluster, for the tests of the checktest
// package. A test follows one pattern:
//
//	env := suite.Setup(t, options...)                      // the cluster and the check
//	env.Delete(t, object)                                  // act on the cluster or the check
//	actual := env.Pop(t, "deleted b")                      // run the check, take what it sent
//	suite.AssertObjects(t, []string{"deleted b"}, actual)  // compare it with what the test expects
package suite

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	orchestratorcheck "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator"
	configenv "github.com/DataDog/datadog-agent/pkg/config/env"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/redact"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/clustername"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

const clusterID = "d801b2b1-4811-11ea-8618-121d4d0938a3"

// Mode is where the check runs.
type Mode int

const (
	ClusterAgentLeader Mode = iota
	ClusterAgentFollower
	ClusterCheckRunner
)

// Env is the environment of a test: the real orchestrator check, configured against a fake Kubernetes cluster, with
// a recorder of what the check sends.
type Env struct {
	Check        *orchestratorcheck.OrchestratorCheck
	Config       configmodel.BuildableConfig
	Tagger       taggermock.Mock
	Workloadmeta workloadmetamock.Mock

	cluster   *fakeCluster
	leader    atomic.Bool
	recorder  *recorder
	pipelines []pipeline
	options   options
	// electionErr is the error of the leader election, other than not leading.
	electionErr atomic.Pointer[error]
	// elections counts the leader elections that the check ran.
	elections atomic.Int32
	// stopped reports that the check was canceled.
	stopped bool
	// deletions are the UIDs of the objects whose deletion the check handled, which WaitForDeletions waits for.
	deletions sync.Map
}

// pipeline is a producer of payloads that the environment runs, stops and records. The orchestrator check is the only
// one today; a manifest pipeline scheduled apart from the check would be a second.
type pipeline struct {
	run    func() error
	cancel func()
}

type options struct {
	mode                Mode
	objects             []runtime.Object
	customObjects       bool
	instance            *string
	crdCollectorsOnly   bool
	settings            map[string]any
	older               bool
	discoveryErr        error
	failedGroupVersions []string
	undiscoveredCRs     bool
	initConfig          string
	globalTags          [4][]string
	noSkipElection      bool
	forbidden           []string
	failingLists        []string
	instanceTags        []string
	withoutCollectors   []string
	extraSyncTimeout    int
	extraObjects        []runtime.Object
}

// Option customizes the environment that Setup creates.
type Option func(*options)

// WithObjects starts the cluster with the objects, instead of an object of every resource.
func WithObjects(objects ...runtime.Object) Option {
	return func(o *options) { o.objects, o.customObjects = append(o.objects, objects...), true }
}

// WithExtraObjects adds objects to the cluster, with the objects of every resource or of WithObjects, before the check
// is configured. API discovery reports the custom resources among them.
func WithExtraObjects(objects ...runtime.Object) Option {
	return func(o *options) { o.extraObjects = append(o.extraObjects, objects...) }
}

// WithMode runs the check as a Cluster Agent leader (the default), a follower, or a cluster check runner.
func WithMode(mode Mode) Option { return func(o *options) { o.mode = mode } }

// WithCollectors selects the collectors of the check by name, as the instance configuration does.
func WithCollectors(names ...string) Option {
	return WithInstance("collectors:\n  - " + strings.Join(names, "\n  - ") + "\n")
}

// WithInstance replaces the instance configuration of the check, which selects every built-in collector by default.
// An empty instance leaves the selection to the check, which selects the stable collectors that discovery reports.
func WithInstance(instance string) Option { return func(o *options) { o.instance = &instance } }

// WithCRDCollectorsOnly selects no built-in collector, and a CRD collector for each built-in custom resource.
func WithCRDCollectorsOnly() Option { return func(o *options) { o.crdCollectorsOnly = true } }

// WithSetting sets an Agent setting.
func WithSetting(key string, value any) Option {
	return func(o *options) {
		if o.settings == nil {
			o.settings = map[string]any{}
		}
		o.settings[key] = value
	}
}

// WithOlderVersions makes the cluster serve the last version of each resource that has several, instead of the first
// one, like a cluster with older operators.
func WithOlderVersions() Option { return func(o *options) { o.older = true } }

// WithDiscoveryFailure makes API discovery fail from the start, until Env.FailDiscovery(nil).
func WithDiscoveryFailure(err error) Option { return func(o *options) { o.discoveryErr = err } }

// WithFailedGroupVersions makes API discovery fail for the group versions only, from the start.
func WithFailedGroupVersions(groupVersions ...string) Option {
	return func(o *options) { o.failedGroupVersions = groupVersions }
}

// WithUndiscoveredCustomResources serves the custom resources of the cluster, but API discovery does not report them
// until Env.InstallCRDs.
func WithUndiscoveredCustomResources() Option { return func(o *options) { o.undiscoveredCRs = true } }

// WithInstanceTags adds tags to the instance configuration, like the tags of a dispatched cluster check.
func WithInstanceTags(tags ...string) Option {
	return func(o *options) { o.instanceTags = append(o.instanceTags, tags...) }
}

// WithoutCollectors removes collectors from the default selection of every built-in collector.
func WithoutCollectors(names ...string) Option {
	return func(o *options) { o.withoutCollectors = append(o.withoutCollectors, names...) }
}

// WithExtraSyncTimeout sets extra_sync_timeout_seconds in the instance configuration.
func WithExtraSyncTimeout(seconds int) Option {
	return func(o *options) { o.extraSyncTimeout = seconds }
}

// WithInitConfig sets the init_config of the check configuration.
func WithInitConfig(initConfig string) Option { return func(o *options) { o.initConfig = initConfig } }

// WithGlobalTags makes the tagger report the global tags, like the tags of the Agent configuration, before the check is
// configured.
func WithGlobalTags(tags ...string) Option { return func(o *options) { o.globalTags[0] = tags } }

// WithGlobalTagsByCardinality makes the tagger report global tags of each cardinality before the check is configured.
func WithGlobalTagsByCardinality(low, orchestrator, high, standard []string) Option {
	return func(o *options) { o.globalTags = [4][]string{low, orchestrator, high, standard} }
}

// WithoutSkipLeaderElection leaves skip_leader_election out of the configuration of a cluster check runner, like a
// cluster check dispatched without it.
func WithoutSkipLeaderElection() Option { return func(o *options) { o.noSkipElection = true } }

// WithForbiddenResources makes the API server forbid listing the custom resources of the collectors, like
// argoproj.io/v1alpha1/rollouts.
func WithForbiddenResources(collectors ...string) Option {
	return func(o *options) { o.forbidden = append(o.forbidden, collectors...) }
}

// WithFailingLists makes every list of the built-in resources of the collectors fail, like apps/v1/deployments, so that
// their informers never sync.
func WithFailingLists(collectors ...string) Option {
	return func(o *options) { o.failingLists = append(o.failingLists, collectors...) }
}

// Setup returns the environment of a test, which options customize. By default, the cluster has an object of every
// resource, and the check runs as the Cluster Agent leader with every built-in collector.
func Setup(t *testing.T, opts ...Option) *Env {
	var o options
	for _, opt := range opts {
		opt(&o)
	}

	resetResourceCache(t)
	redact.RestoreSensitiveAnnotationsAndLabelsForTest(t)
	configenv.SetFeatures(t, configenv.Kubernetes)
	clustername.ResetClusterName()
	clustername.DisableNodeLabelLookupForTest(t)
	t.Setenv("DD_ORCHESTRATOR_CLUSTER_ID", clusterID)
	config := mockconfig.New(t)
	settings := map[string]any{
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
		// Every built-in custom resource family is enabled, so that each custom resource can be collected.
		"orchestrator_explorer.custom_resources.ootb.enabled":             true,
		"orchestrator_explorer.custom_resources.ootb.gateway_api":         true,
		"orchestrator_explorer.custom_resources.ootb.service_mesh":        true,
		"orchestrator_explorer.custom_resources.ootb.ingress_controllers": true,
	}
	maps.Copy(settings, o.settings)
	for key, value := range settings {
		config.SetInTest(key, value)
	}

	env := &Env{
		Config:       config,
		Tagger:       taggerfxmock.SetupFakeTagger(t),
		Workloadmeta: fxutil.Test[workloadmetamock.Mock](t, fx.Options(core.MockBundle(), workloadmetafxmock.MockModule(workloadmeta.NewParams()))),
		cluster:      newFakeCluster(t),
	}
	env.options = o
	env.Tagger.SetGlobalTags(o.globalTags[0], o.globalTags[1], o.globalTags[2], o.globalTags[3])
	for _, collector := range o.forbidden {
		env.cluster.forbid(gvrOf(t, collector))
	}
	for _, collector := range o.failingLists {
		env.cluster.failLists(gvrOf(t, collector))
	}
	env.leader.Store(o.mode != ClusterAgentFollower)
	env.cluster.failDiscovery(o.discoveryErr)
	env.cluster.failGroupVersions(o.failedGroupVersions)
	// Like a real cluster, discovery reports every built-in resource, whatever the objects of the cluster.
	for _, f := range servedVersions(builtinFixtures(t), o.older) {
		if f.Discovery != nil {
			env.cluster.discover(f.groupVersion, *f.Discovery)
		}
	}

	var objects []Fixture
	if o.customObjects {
		for _, object := range o.objects {
			objects = append(objects, fixtureOf(object))
		}
	} else {
		objects = append(servedVersions(builtinFixtures(t), o.older), servedVersions(customResourceFixtures(), o.older)...)
	}
	for _, object := range o.extraObjects {
		objects = append(objects, fixtureOf(object))
	}
	for _, f := range objects {
		env.add(t, f)
		if f.isCustomResource() && !o.undiscoveredCRs {
			env.cluster.discover(f.groupVersion, *f.Discovery)
		}
	}
	env.configure(t, o)
	t.Cleanup(func() { env.stop(t) })
	return env
}

func gvrOf(t *testing.T, collector string) schema.GroupVersionResource {
	parts := strings.Split(collector, "/")
	require.GreaterOrEqual(t, len(parts), 2, "%s is not a collector full name", collector)
	gv, err := schema.ParseGroupVersion(strings.Join(parts[:len(parts)-1], "/"))
	require.NoError(t, err)
	return gv.WithResource(parts[len(parts)-1])
}

// servedVersions returns the fixtures of the version of each resource that the cluster serves: the first one, or the
// last one for a cluster with older versions.
func servedVersions(fixtures []Fixture, older bool) []Fixture {
	var served []Fixture
	for _, f := range fixtures {
		i := slices.IndexFunc(served, func(s Fixture) bool { return s.gvr.GroupResource() == f.gvr.GroupResource() })
		switch {
		case i < 0:
			served = append(served, f)
		case older:
			served[i] = f
		}
	}
	return served
}

func (env *Env) configure(t *testing.T, o options) {
	quietLogs(t)
	env.Check = orchestratorcheck.NewCheckForTest(env.Config, env.Workloadmeta, env.Tagger, orchestratorcheck.TestDependencies{
		WaitForAPIClient: func(context.Context) (*apiserver.APIClient, error) { return env.cluster.apiClient, nil },
		RunLeaderElection: func() (string, error) {
			env.elections.Add(1)
			if o.mode == ClusterCheckRunner && !o.noSkipElection {
				t.Error("a cluster check runner that skips the leader election must not run it")
			}
			if err := env.electionErr.Load(); err != nil && *err != nil {
				return "", *err
			}
			if env.leader.Load() {
				return "self", nil
			}
			return "other", apiserver.ErrNotLeader
		},
		ServerGroupsAndResources: env.cluster.serverGroupsAndResources,
		IsCLCRunner:              o.mode == ClusterCheckRunner,
		OnDeletion: func(obj any) {
			if unknown, ok := obj.(cache.DeletedFinalStateUnknown); ok {
				obj = unknown.Obj
			}
			if object, ok := obj.(runtime.Object); ok {
				env.deletions.Store(uidOf(object), true)
			}
		},
	})

	var instance strings.Builder
	if o.mode == ClusterCheckRunner && !o.noSkipElection {
		instance.WriteString("skip_leader_election: true\n")
	}
	switch {
	case o.instance != nil:
		instance.WriteString(*o.instance)
	case o.crdCollectorsOnly:
		instance.WriteString("collectors: []\ncrd_collectors:\n")
		for _, f := range servedVersions(customResourceFixtures(), o.older) {
			instance.WriteString("  - " + f.Collector + "\n")
		}
	default:
		instance.WriteString("collectors:\n")
		for _, name := range builtinCollectorNames(env) {
			if !slices.Contains(o.withoutCollectors, name) {
				instance.WriteString("  - " + name + "\n")
			}
		}
	}
	if o.extraSyncTimeout > 0 {
		fmt.Fprintf(&instance, "extra_sync_timeout_seconds: %d\n", o.extraSyncTimeout)
	}

	if len(o.instanceTags) > 0 {
		instance.WriteString("tags:\n  - " + strings.Join(o.instanceTags, "\n  - ") + "\n")
	}

	senderManager := mocksender.CreateDefaultDemultiplexer(t)
	env.recorder = newRecorder(t, env.Check.ID(), senderManager)
	require.NoError(t, env.Check.Configure(senderManager, 1, integration.Data(instance.String()), integration.Data(o.initConfig), "test", "provider"))
	env.pipelines = nil
	// Configure derives the check ID from its configuration.
	env.addPipeline(env.Check.ID(), env.Check.Run, env.Check.Cancel)
	env.stopped = false
}

// addPipeline records what a producer sends through the sender of its check ID, and runs it at each check run.
func (env *Env) addPipeline(id checkid.ID, run func() error, cancel func()) {
	mocksender.SetSender(env.recorder.sender, id)
	env.pipelines = append(env.pipelines, pipeline{run: run, cancel: cancel})
}

// stop cancels the check, which flushes the deletions that it queued.
func (env *Env) stop(t *testing.T) {
	if env.stopped {
		return
	}
	env.stopped = true
	for _, p := range env.pipelines {
		_ = runWithTimeout(t, func() error { p.cancel(); return nil })
	}
}

// Stop cancels the check, like the Agent does when it unschedules it, and returns what the check sent meanwhile.
func (env *Env) Stop(t *testing.T) Output {
	t.Helper()
	env.stop(t)
	out := env.recorder.take()
	env.assertNoSensitiveData(t, out)
	return out
}

// Restart cancels the check and configures a new one in the same process, like the Agent does when it reschedules
// it. The cache of the resource versions that the check sent is kept, as it is shared by the process.
func (env *Env) Restart(t *testing.T) {
	t.Helper()
	env.stop(t)
	env.configure(t, env.options)
}

// WaitForDeletions waits until the check has handled the deletion of each object, like "deleted <uid>", whether it
// queued it or not, for a step that must follow it, like Stop or a change of leadership.
func (env *Env) WaitForDeletions(t *testing.T, objects ...string) {
	t.Helper()
	missing := func() []string {
		var missing []string
		for _, object := range objects {
			if _, found := env.deletions.Load(strings.TrimPrefix(object, "deleted ")); !found {
				missing = append(missing, object)
			}
		}
		return missing
	}
	if !assert.Eventually(t, func() bool { return len(missing()) == 0 }, 10*time.Second, time.Millisecond) {
		require.FailNow(t, "the check did not handle the deletions", "%v", missing())
	}
}

// ExpireResourceCache expires the cache of the resource versions that the check sent, as it does three minutes after
// an object last changed. The check then sends every object again.
func (env *Env) ExpireResourceCache() { orchestrator.KubernetesResourceCache.Flush() }

// FailLeaderElection makes the leader election return the error, or work again when it is nil.
func (env *Env) FailLeaderElection(err error) { env.electionErr.Store(&err) }

// LeaderElections returns the number of leader elections that the check ran.
func (env *Env) LeaderElections() int { return int(env.elections.Load()) }

// builtinCollectorNames returns the names of the built-in collectors, which the default instance selects.
func builtinCollectorNames(env *Env) []string {
	var names []string
	for _, m := range orchestratorcheck.CollectorMetadataForTest(env.Config, env.Workloadmeta, env.Tagger) {
		if !slices.Contains(names, m.Name) {
			names = append(names, m.Name)
		}
	}
	return names
}

// Objects returns the objects of the cluster, in their current state.
func (env *Env) Objects() []runtime.Object {
	objects := make([]runtime.Object, 0, len(env.cluster.objects))
	for _, f := range env.cluster.objects {
		objects = append(objects, f.Object.DeepCopyObject())
	}
	return objects
}

// AddCheck configures another check in the same process and cluster, like a second instance on a cluster check
// runner. The options change the ones of Setup, except the objects.
func (env *Env) AddCheck(t *testing.T, opts ...Option) *Env {
	o := env.options
	o.instance, o.crdCollectorsOnly, o.instanceTags = nil, false, nil
	for _, opt := range opts {
		opt(&o)
	}
	other := &Env{
		Config:       env.Config,
		Tagger:       env.Tagger,
		Workloadmeta: env.Workloadmeta,
		cluster:      env.cluster,
		options:      o,
	}
	other.leader.Store(o.mode != ClusterAgentFollower)
	other.configure(t, o)
	t.Cleanup(func() { other.stop(t) })
	return other
}

// SetEntityTags makes the tagger report the tags for every object of the cluster, as it does for the Kubernetes
// metadata that workloadmeta collects. The processors of most resources add them to the tags of their model.
func (env *Env) SetEntityTags(tags ...string) {
	for _, f := range env.cluster.objects {
		env.Tagger.SetTags(entityIDOf(f), "test", tags, nil, nil, nil)
	}
}

// entityIDOf returns the tagger entity of an object, as its processor looks it up.
func entityIDOf(f Fixture) taggertypes.EntityID {
	accessor, _ := meta.Accessor(f.Object)
	switch f.Object.(type) {
	case *appsv1.Deployment:
		return taggertypes.NewEntityID(taggertypes.KubernetesDeployment, accessor.GetNamespace()+"/"+accessor.GetName())
	case *corev1.Node:
		return taggertypes.NewEntityID(taggertypes.KubernetesNode, accessor.GetName())
	}
	return taggertypes.NewEntityID(taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(f.gvr.Group, f.gvr.Resource, accessor.GetNamespace(), accessor.GetName())))
}

// AddCopies adds a copy of every object of the cluster, with its own name and UID, and returns the UIDs of the copies.
func (env *Env) AddCopies(t *testing.T) []string {
	var uids []string
	for _, f := range slices.Clone(env.cluster.objects) {
		accessor, err := meta.Accessor(f.Object)
		require.NoError(t, err)
		c := f
		c.Object = f.Object.DeepCopyObject()
		copied, err := meta.Accessor(c.Object)
		require.NoError(t, err)
		copied.SetName(accessor.GetName() + "-copy")
		copied.SetUID(accessor.GetUID() + "-copy")
		copied.SetResourceVersion("1")
		env.add(t, c)
		uids = append(uids, string(copied.GetUID()))
	}
	return uids
}

// DeleteUIDs deletes the objects of the cluster with the UIDs.
func (env *Env) DeleteUIDs(t *testing.T, uids ...string) {
	for _, object := range env.Objects() {
		if slices.Contains(uids, uidOf(object)) {
			env.Delete(t, object)
		}
	}
}

// Object returns the object of the cluster with the UID, in its current state.
func (env *Env) Object(t *testing.T, uid string) runtime.Object {
	i := slices.IndexFunc(env.cluster.objects, func(f Fixture) bool { return uidOf(f.Object) == uid })
	require.GreaterOrEqual(t, i, 0, "%s is not an object of the cluster", uid)
	return env.cluster.objects[i].Object.DeepCopyObject()
}

// CustomResourceUIDs returns the UIDs of the custom resources of the cluster.
func (env *Env) CustomResourceUIDs() []string {
	var uids []string
	for _, f := range env.cluster.objects {
		if f.isCustomResource() {
			uids = append(uids, uidOf(f.Object))
		}
	}
	return uids
}

// UIDs returns the UIDs of the objects of the cluster.
func (env *Env) UIDs() []string {
	uids := make([]string, 0, len(env.cluster.objects))
	for _, f := range env.cluster.objects {
		uids = append(uids, uidOf(f.Object))
	}
	return uids
}

// Add adds an object to the cluster.
func (env *Env) Add(t *testing.T, object runtime.Object) { env.add(t, fixtureOf(object)) }

func (env *Env) add(t *testing.T, f Fixture) {
	// The cluster and the test must not share the object.
	f.Object = f.Object.DeepCopyObject()
	env.cluster.add(t, f.Object.DeepCopyObject())
	env.cluster.objects = append(env.cluster.objects, f)
}

// Update replaces an object of the cluster.
func (env *Env) Update(t *testing.T, object runtime.Object) {
	env.cluster.update(t, object)
	env.cluster.objects[env.indexOf(t, object)].Object = object.DeepCopyObject()
}

// Delete deletes an object of the cluster.
func (env *Env) Delete(t *testing.T, object runtime.Object) {
	env.cluster.delete(t, object)
	env.cluster.objects = slices.Delete(env.cluster.objects, env.indexOf(t, object), env.indexOf(t, object)+1)
}

// UpdateAll adds a label to every object of the cluster, with the next resource version.
func (env *Env) UpdateAll(t *testing.T) {
	for _, object := range env.Objects() {
		env.Update(t, Updated(t, object))
	}
}

// DeleteAll deletes every object of the cluster.
func (env *Env) DeleteAll(t *testing.T) {
	for _, object := range env.Objects() {
		env.Delete(t, object)
	}
}

func (env *Env) indexOf(t *testing.T, object runtime.Object) int {
	i := slices.IndexFunc(env.cluster.objects, func(f Fixture) bool { return uidOf(f.Object) == uidOf(object) })
	require.GreaterOrEqual(t, i, 0, "%s is not an object of the cluster", uidOf(object))
	return i
}

// SetLeader sets the result of the leader election of the next runs.
func (env *Env) SetLeader(leader bool) { env.leader.Store(leader) }

// FailDiscovery makes API discovery return the error, or work again when it is nil.
func (env *Env) FailDiscovery(err error) { env.cluster.failDiscovery(err) }

// FailGroupVersions makes API discovery fail for the group versions only. No group version makes it work again.
func (env *Env) FailGroupVersions(groupVersions ...string) {
	env.cluster.failGroupVersions(groupVersions)
}

// Discover makes API discovery report the resource of a custom resource object, as the API server does once its CRD
// is established.
func (env *Env) Discover(t *testing.T, object runtime.Object) {
	f := fixtureOf(object)
	require.NotNil(t, f.Discovery, "%T %s is not a known custom resource", object, uidOf(object))
	env.cluster.discover(f.groupVersion, *f.Discovery)
}

// InstallCRDs makes API discovery report each custom resource of the cluster, and adds its CRD.
func (env *Env) InstallCRDs(t *testing.T) {
	for _, f := range slices.Clone(env.cluster.objects) {
		if f.isCustomResource() {
			env.cluster.discover(f.groupVersion, *f.Discovery)
			env.Add(t, crdOf(f))
		}
	}
}

// SetInfraTags makes the tagger report the infrastructure mode tags, as the Cluster Agent tagger stream does.
func (env *Env) SetInfraTags(tags ...string) {
	env.Tagger.SetTags(taggertypes.GetInfraTagsEntityID(), "remote", tags, nil, nil, nil)
}

// Pop runs the check until it has sent each object, like "live <uid>", and returns everything that it sent. A step that
// expects nothing after a change first waits for a signal that the check handled it, like WaitForDeletions.
func (env *Env) Pop(t *testing.T, objects ...string) Output {
	t.Helper()
	out, err := env.pop(t, objects)
	require.NoError(t, err, "a check run failed")
	return out
}

// PopWithError is Pop with no object, for a check whose runs fail. It returns the error of the first run.
func (env *Env) PopWithError(t *testing.T) (Output, error) {
	t.Helper()
	return env.pop(t, nil)
}

func (env *Env) pop(t *testing.T, objects []string) (Output, error) {
	t.Helper()
	return env.runUntil(t, func(out Output) bool { return containsAll(out.Objects(), objects) })
}

// runUntil runs the check until the condition holds on everything sent, or for 10 seconds, as informers report changes
// asynchronously. It then runs it once more so that late duplicates show up, and returns the first error of the runs.
func (env *Env) runUntil(t *testing.T, done func(Output) bool) (Output, error) {
	t.Helper()
	var out Output
	var firstErr error
	run := func() {
		sent, err := env.runOnce(t)
		out = append(out, sent...)
		if firstErr == nil {
			firstErr = err
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		run()
		if done(out) || time.Now().After(deadline) {
			time.Sleep(30 * time.Millisecond)
			run()
			return out, firstErr
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (env *Env) runOnce(t *testing.T) (Output, error) {
	t.Helper()
	var errs []error
	for _, p := range env.pipelines {
		errs = append(errs, runWithTimeout(t, p.run))
	}
	out := env.recorder.take()
	env.assertNoSensitiveData(t, out)
	return out, errors.Join(errs...)
}

// Updated returns a copy of the object with a new label and the next resource version.
func Updated(t *testing.T, object runtime.Object) runtime.Object {
	object = object.DeepCopyObject()
	accessor, err := meta.Accessor(object)
	require.NoError(t, err)
	labels := accessor.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	labels["updated"] = "true"
	accessor.SetLabels(labels)
	accessor.SetResourceVersion(nextResourceVersion(accessor.GetResourceVersion()))
	return object
}

func uidOf(object runtime.Object) string {
	accessor, err := meta.Accessor(object)
	if err != nil {
		return ""
	}
	return string(accessor.GetUID())
}

// fixtureOf returns the fixture of an object of a test. A custom resource keeps what discovery reports for its
// resource.
func fixtureOf(object runtime.Object) Fixture {
	if u, ok := object.(*unstructured.Unstructured); ok {
		for _, f := range append(customResourceFixtures(), userCustomResourceFixtures()...) {
			if f.gvr == resourceOf(u.GroupVersionKind()) {
				f.Object = object
				return f
			}
		}
	}
	f := Fixture{Object: object}
	if kinds, _, err := fixtureCodecs().scheme.ObjectKinds(object); err == nil {
		f.gvr, _ = meta.UnsafeGuessKindToResource(kinds[0])
	}
	return f
}

// runWithTimeout returns the error of the function, and fails the test when the function blocks, as a blocked check
// run would otherwise hang the test. The function runs in another goroutine, so it must not fail the test itself.
func runWithTimeout(t *testing.T, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(10 * time.Second):
		t.Fatal("the check is blocked")
		return nil
	}
}

func containsAll(got, want []string) bool {
	for _, w := range want {
		if !slices.Contains(got, w) {
			return false
		}
	}
	return true
}

func sortedCopy(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return slices.Sorted(slices.Values(s))
}

// resetResourceCache empties the cache of the resource versions that the check sent, and restores it when the test
// ends.
func resetResourceCache(t *testing.T) {
	resourceCache := orchestrator.KubernetesResourceCache
	t.Cleanup(func() { orchestrator.KubernetesResourceCache = resourceCache })
	orchestrator.KubernetesResourceCache = orchestrator.NewKubernetesResourceCache()
}

// quietLogs replaces the trace logger that the component mocks install with a warning one, as the tests run the check
// many times and debug logs slow them down.
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
