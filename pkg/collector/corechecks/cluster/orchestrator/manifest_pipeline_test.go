// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package orchestrator

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"go.uber.org/fx"
	appsv1 "k8s.io/api/apps/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/core"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
	corechecks "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/inventory"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors/k8s"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/registry"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchcfg "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TestManifestCollectorParity checks that each collector of the manifest pipeline sends the same manifests as the
// collector of the combined pipeline for the same resource, with CR and CRD manifests in regular payloads.
func TestManifestCollectorParity(t *testing.T) {
	cfg := mockconfig.New(t)
	store := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		core.MockBundle(),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.KubernetesDeployment, "default/name"), "workloadmeta", []string{"team:a"}, nil, nil, nil)

	combinedInventory := inventory.NewCollectorInventory(cfg, store, fakeTagger)
	manifestInventory := inventory.NewManifestCollectorInventory(cfg, store, fakeTagger)

	for collectorName, fixture := range registry.ResourceFixtures() {
		object := fixture.Object
		t.Run(collectorName, func(t *testing.T) {
			combinedCollector := collectorByName(t, combinedInventory, collectorName)
			manifestCollector := collectorByName(t, manifestInventory, collectorName)
			assert.IsType(t, &k8s.ResourceCollector{}, manifestCollector)

			manifests := assertSameManifests(t, combinedCollector, manifestCollector, object)
			if collectorName == "apps/v1/deployments" {
				assert.Contains(t, manifests[0].Tags, "team:a")
			}
		})
	}

	t.Run("custom resource", func(t *testing.T) {
		crCollector, err := k8s.NewCRCollector("datadogmetrics", "datadoghq.com/v1alpha1")
		require.NoError(t, err)
		crCollector.Metadata().Kind = "DatadogMetric"
		cr := &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "datadoghq.com/v1alpha1",
			"kind":       "DatadogMetric",
			"metadata":   map[string]interface{}{"name": "name", "namespace": "default", "uid": "uid", "resourceVersion": "1"},
			"spec":       map[string]interface{}{"query": "avg:system.cpu.user{*}"},
		}}

		assertSameManifests(t, crCollector, crCollector.ForManifests(), cr)
	})
}

// assertSameManifests processes a resource with both collectors, compares their manifests and returns them. It
// checks the manifests of the resource as a live and as a terminated resource.
func assertSameManifests(t *testing.T, combinedCollector, manifestCollector collectors.K8sCollector, object interface{}) []*model.Manifest {
	t.Run("terminated", func(t *testing.T) {
		combined := processWithEmptyCaches(t, combinedCollector, []interface{}{object}, true)
		dedicated := processWithEmptyCaches(t, manifestCollector, []interface{}{object}, true)

		combinedManifests := manifestsOf(t, combined.ManifestMessages)
		dedicatedManifests := manifestsOf(t, dedicated.ManifestMessages)
		require.Len(t, dedicatedManifests, 1)
		assert.True(t, dedicatedManifests[0].IsTerminated)
		assert.Equal(t, withoutDeletionTimestamp(t, combinedManifests), withoutDeletionTimestamp(t, dedicatedManifests))
	})

	combined := processWithEmptyCaches(t, combinedCollector, []interface{}{object}, false)
	dedicated := processWithEmptyCaches(t, manifestCollector, []interface{}{object}, false)

	assert.Empty(t, dedicated.MetadataMessages)
	require.Len(t, dedicated.ManifestMessages, 1)
	assert.IsType(t, &model.CollectorManifest{}, dedicated.ManifestMessages[0])
	manifests := manifestsOf(t, dedicated.ManifestMessages)
	assert.Equal(t, manifestsOf(t, combined.ManifestMessages), manifests)
	return manifests
}

func collectorByName(t *testing.T, collectorInventory *inventory.CollectorInventory, fullName string) collectors.K8sCollector {
	i := strings.LastIndex(fullName, "/")
	collector, err := collectorInventory.CollectorForVersion(fullName[i+1:], fullName[:i])
	require.NoError(t, err)
	return collector
}

// processWithEmptyCaches processes the resources with empty resource caches.
func processWithEmptyCaches(t *testing.T, collector collectors.K8sCollector, list interface{}, terminated bool) processors.ProcessResult {
	resetResourceCaches(t)

	cfg := orchcfg.NewDefaultOrchestratorConfig([]string{"extra:tag"})
	cfg.IsManifestCollectionEnabled = true
	cfg.IsScrubbingEnabled = true
	cfg.KubeClusterName = "cluster"
	result, err := collector.Process(&collectors.CollectorRunConfig{
		ClusterID:           "cluster-id",
		Config:              cfg,
		MsgGroupRef:         atomic.NewInt32(0),
		TerminatedResources: terminated,
	}, list)
	require.NoError(t, err)
	require.Equal(t, 1, result.ResourcesProcessed)
	return result.Result
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

// withoutDeletionTimestamp returns copies of terminated manifests whose content has no deletion timestamp, as each
// processing run sets it to its own time. It checks that the timestamp is set.
func withoutDeletionTimestamp(t *testing.T, manifests []*model.Manifest) []*model.Manifest {
	normalized := make([]*model.Manifest, 0, len(manifests))
	for _, manifest := range manifests {
		var content map[string]interface{}
		require.NoError(t, json.Unmarshal(manifest.Content, &content))
		metadata := content["metadata"].(map[string]interface{})
		require.Contains(t, metadata, "deletionTimestamp")
		delete(metadata, "deletionTimestamp")

		copied := *manifest
		// The version of a pod is a hash of its model, which includes the deletion time.
		if manifest.Kind == "Pod" {
			copied.ResourceVersion = ""
		}
		var err error
		copied.Content, err = json.Marshal(content)
		require.NoError(t, err)
		normalized = append(normalized, &copied)
	}
	return normalized
}

// manifestsOf returns the manifests of manifest payloads, unwrapping CR and CRD payloads.
func manifestsOf(t *testing.T, messages []model.MessageBody) []*model.Manifest {
	var manifests []*model.Manifest
	for _, message := range messages {
		switch m := message.(type) {
		case *model.CollectorManifest:
			manifests = append(manifests, m.Manifests...)
		case *model.CollectorManifestCR:
			manifests = append(manifests, m.Manifest.Manifests...)
		case *model.CollectorManifestCRD:
			manifests = append(manifests, m.Manifest.Manifests...)
		default:
			t.Fatalf("unexpected manifest payload type %T", message)
		}
	}
	return manifests
}

func TestCollectorBundleRoleSelection(t *testing.T) {
	fetch := func() ([]*metav1.APIGroup, []*metav1.APIResourceList, error) {
		return []*metav1.APIGroup{
				{
					Name:             "apps",
					Versions:         []metav1.GroupVersionForDiscovery{{GroupVersion: "apps/v1", Version: "v1"}},
					PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "apps/v1", Version: "v1"},
				},
				{
					Name:             "apiextensions.k8s.io",
					Versions:         []metav1.GroupVersionForDiscovery{{GroupVersion: "apiextensions.k8s.io/v1", Version: "v1"}},
					PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "apiextensions.k8s.io/v1", Version: "v1"},
				},
				{
					Name:             "datadoghq.com",
					Versions:         []metav1.GroupVersionForDiscovery{{GroupVersion: "datadoghq.com/v1alpha1", Version: "v1alpha1"}},
					PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "datadoghq.com/v1alpha1", Version: "v1alpha1"},
				},
			}, []*metav1.APIResourceList{
				{GroupVersion: "apps/v1", APIResources: []metav1.APIResource{{Name: "deployments", Kind: "Deployment"}}},
				{GroupVersion: "apiextensions.k8s.io/v1", APIResources: []metav1.APIResource{{Name: "customresourcedefinitions", Kind: "CustomResourceDefinition"}}},
				{GroupVersion: "datadoghq.com/v1alpha1", APIResources: []metav1.APIResource{{Name: "datadogmetrics", Kind: "DatadogMetric"}}},
			}, nil
	}
	settings := map[string]any{"orchestrator_explorer.custom_resources.ootb.enabled": false}
	allCollectors := []string{"apiextensions.k8s.io/v1/customresourcedefinitions", "apps/v1/deployments", "datadoghq.com/v1alpha1/datadogmetrics"}

	for _, tc := range []struct {
		role     collectors.BundleRole
		expected []string
	}{
		{role: collectors.MetadataAndManifestRole, expected: allCollectors},
		{role: collectors.MetadataRole, expected: []string{"apps/v1/deployments"}},
		{role: collectors.ManifestRole, expected: allCollectors},
	} {
		cb := newRetryTestBundle(t, []string{"apps/v1/deployments", "apiextensions.k8s.io/v1/customresourcedefinitions"}, false, fetch, settings)
		if tc.role == collectors.ManifestRole {
			cb.inventory = inventory.NewManifestCollectorInventory(mockconfig.New(t), nil, nil)
		}
		cb.check.instance.CRDCollectors = []string{"datadoghq.com/v1alpha1/datadogmetrics"}
		cb.runCfg.Role = tc.role

		cb.prepare()

		assert.Equal(t, tc.expected, collectorNames(cb))
		if tc.role == collectors.ManifestRole {
			for _, collector := range cb.collectors {
				assert.IsType(t, &k8s.ResourceCollector{}, collector)
			}
			crIndex := slices.IndexFunc(cb.collectors, func(c collectors.K8sCollector) bool { return c.Metadata().Name == "datadogmetrics" })
			assert.Equal(t, "DatadogMetric", cb.collectors[crIndex].Metadata().Kind)
		}
	}
}

func TestCollectorBundleRunRoles(t *testing.T) {
	for _, tc := range []struct {
		role              collectors.BundleRole
		expectedManifests int
	}{
		{role: collectors.MetadataAndManifestRole, expectedManifests: 1},
		{role: collectors.MetadataRole, expectedManifests: 0},
	} {
		sender := mocksender.NewMockSender(t, checkid.ID("orchestrator"))
		sender.On("OrchestratorMetadata", mock.Anything, mock.Anything, mock.Anything).Return()
		sender.On("OrchestratorManifest", mock.Anything, mock.Anything).Return()

		chk := &OrchestratorCheck{
			CheckBase:          corechecks.NewCheckBase(CheckName),
			orchestratorConfig: &orchcfg.OrchestratorConfig{IsManifestCollectionEnabled: true},
			groupID:            atomic.NewInt32(0),
		}
		runCfg := &collectors.CollectorRunConfig{Config: chk.orchestratorConfig, Role: tc.role}
		cb := &CollectorBundle{
			check:          chk,
			collectors:     []collectors.K8sCollector{&roleTestCollector{}},
			runCfg:         runCfg,
			manifestBuffer: NewManifestBuffer(chk),
			terminatedGate: &collectors.TerminatedResourceGate{},
		}

		cb.Run(sender)

		sender.AssertNumberOfCalls(t, "OrchestratorMetadata", 1)
		sender.AssertNumberOfCalls(t, "OrchestratorManifest", tc.expectedManifests)
	}
}

// TestManifestBundleSendsTerminatedManifests checks that the manifest bundle sends the manifest of a deleted resource,
// with and without the manifest buffer.
func TestManifestBundleSendsTerminatedManifests(t *testing.T) {
	for _, buffered := range []bool{true, false} {
		t.Run(fmt.Sprintf("buffered=%t", buffered), func(t *testing.T) {
			testManifestBundleSendsTerminatedManifests(t, buffered)
		})
	}
}

func testManifestBundleSendsTerminatedManifests(t *testing.T, buffered bool) {
	resetResourceCaches(t)

	checkBase := corechecks.NewCheckBase(CheckName)
	sender := mocksender.NewMockSender(t, checkBase.ID())
	var sent []*model.CollectorManifest
	sender.On("OrchestratorManifest", mock.Anything, mock.Anything).Return().Run(func(args mock.Arguments) {
		for _, message := range args.Get(0).([]model.MessageBody) {
			sent = append(sent, message.(*model.CollectorManifest))
		}
	})
	sender.On("OrchestratorMetadata", mock.Anything, mock.Anything, mock.Anything).Return()

	cfg := orchcfg.NewDefaultOrchestratorConfig(nil)
	cfg.IsManifestCollectionEnabled = true
	cfg.BufferedManifestEnabled = buffered
	cfg.ManifestBufferFlushInterval = time.Minute
	cfg.KubeClusterName = "cluster"
	chk := &OrchestratorCheck{CheckBase: checkBase, orchestratorConfig: cfg, groupID: atomic.NewInt32(0), clusterID: "cluster-id"}
	require.NoError(t, chk.CommonConfigure(sender.GetSenderManager(), nil, nil, "test", "provider"))

	collector := collectorByName(t, inventory.NewManifestCollectorInventory(mockconfig.New(t), nil, nil), "apiextensions.k8s.io/v1/customresourcedefinitions")
	runCfg := &collectors.CollectorRunConfig{ClusterID: chk.clusterID, Config: cfg, MsgGroupRef: chk.groupID, Role: collectors.ManifestRole}
	runCfg.TerminatedResourceGate = &collectors.TerminatedResourceGate{}
	runCfg.TerminatedResourceGate.Enable()
	cb := &CollectorBundle{
		check:                chk,
		runCfg:               runCfg,
		manifestBuffer:       NewManifestBuffer(chk),
		terminatedGate:       runCfg.TerminatedResourceGate,
		terminatedCollectors: []collectors.K8sCollector{collector},
	}

	collector.(*k8s.ResourceCollector).SetTerminatedResourceGate(runCfg.TerminatedResourceGate)
	collector.QueueTerminated(&apiextensionsv1.CustomResourceDefinition{ObjectMeta: metav1.ObjectMeta{Name: "crd", UID: "crd-uid", ResourceVersion: "1"}})
	cb.Run(sender)

	sender.AssertNotCalled(t, "OrchestratorMetadata", mock.Anything, mock.Anything, mock.Anything)
	require.Len(t, sent, 1)
	require.Len(t, sent[0].Manifests, 1)
	manifest := sent[0].Manifests[0]
	assert.True(t, manifest.IsTerminated)
	assert.Equal(t, "crd-uid", manifest.Uid)
	assert.Equal(t, int32(orchestrator.K8sCRD), manifest.Type)
}

func TestOrchestratorCheckRunInformerOnce(t *testing.T) {
	stopCh := make(chan struct{})
	t.Cleanup(func() { close(stopCh) })
	chk := &OrchestratorCheck{runningInformers: map[cache.Store]struct{}{}}
	informer := cache.NewSharedInformer(&cache.ListWatch{}, &appsv1.Deployment{}, 0)

	chk.runInformer(informer, stopCh)
	chk.runInformer(informer, stopCh)

	assert.Len(t, chk.runningInformers, 1)
}

// roleTestCollector returns one metadata and one manifest message on each run.
type roleTestCollector struct {
	collectors.TerminatedQueue
}

func (c *roleTestCollector) Init(*collectors.CollectorRunConfig) {}

func (c *roleTestCollector) Informer() cache.SharedInformer { return nil }

func (c *roleTestCollector) Metadata() *collectors.CollectorMetadata {
	return &collectors.CollectorMetadata{IsMetadataProducer: true, IsManifestProducer: true, Name: "deployments", NodeType: orchestrator.K8sDeployment}
}

func (c *roleTestCollector) Run(rcfg *collectors.CollectorRunConfig) (*collectors.CollectorRunResult, error) {
	return c.Process(rcfg, nil)
}

func (c *roleTestCollector) Process(*collectors.CollectorRunConfig, interface{}) (*collectors.CollectorRunResult, error) {
	return &collectors.CollectorRunResult{
		Result: processors.ProcessResult{
			MetadataMessages: []model.MessageBody{&model.CollectorDeployment{}},
			ManifestMessages: []model.MessageBody{&model.CollectorManifest{}},
		},
	}, nil
}
