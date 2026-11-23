// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package k8s

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processorstest"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/registry"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// TestResourceCollectors runs the collector of each version of each built-in registry entry, and of a discovered
// custom resource, on a resource of its kind. It runs each collector in the combined and in the manifest pipeline, as
// a listed and as a terminated resource.
func TestResourceCollectors(t *testing.T) {
	d := registry.Dependencies{
		Config: mockconfig.New(t),
		Store: fxutil.Test[workloadmetamock.Mock](t, fx.Options(
			core.MockBundle(),
			workloadmetafxmock.MockModule(workloadmeta.NewParams()),
		)),
		Tagger: processorstest.NewEmptyFakeTagger(),
	}
	fixtures := registry.ResourceFixtures()

	crCollector := newTestCRCollector(t)
	resourceCollectors := map[bool][]*ResourceCollector{false: {crCollector}, true: {crCollector.ForManifests()}}
	fixtures[crCollector.Metadata().FullName()] = registry.ResourceFixture{Object: testCustomResource("name")}
	for _, manifestsOnly := range []bool{false, true} {
		for _, versions := range NewResourceCollectorVersions(d, manifestsOnly) {
			for _, k8sCollector := range versions.Collectors {
				// The cluster collector has its own test.
				if collector, ok := k8sCollector.(*ResourceCollector); ok {
					resourceCollectors[manifestsOnly] = append(resourceCollectors[manifestsOnly], collector)
				}
			}
		}
	}

	for manifestsOnly, collectorList := range resourceCollectors {
		for _, collector := range collectorList {
			name := collector.Metadata().FullName()
			t.Run(fmt.Sprintf("%s/manifestsOnly=%t", name, manifestsOnly), func(t *testing.T) {
				fixture, found := fixtures[name]
				require.True(t, found, "no fixture for %s", name)
				hasMetadata := !manifestsOnly && fixture.MetadataType != nil
				assert.Equal(t, hasMetadata, collector.Metadata().IsMetadataProducer)

				runCfg, start := registry.NewTestRunConfig(t, []runtime.Object{fixture.Object})
				collector.Init(runCfg)
				start()

				// The terminated pods collector has no informer: it only processes deleted pods.
				if informer := collector.Informer(); informer != nil {
					resetResourceCaches(t)
					require.True(t, cache.WaitForCacheSync(runCfg.StopCh, informer.HasSynced))
					result, err := collector.Run(runCfg)
					require.NoError(t, err)
					assert.Equal(t, 1, result.ResourcesListed)
					assert.Equal(t, 1, result.ResourcesProcessed)
					assertResourceResult(t, collector, fixture, result, hasMetadata, manifestsOnly, false)
				}

				resetResourceCaches(t)
				runCfg.TerminatedResources = true
				result, err := collector.Process(runCfg, []interface{}{fixture.Object})
				require.NoError(t, err)
				assertResourceResult(t, collector, fixture, result, hasMetadata, manifestsOnly, true)
			})
		}
	}
}

// TestCRCollectorQuota checks that a custom resource collector, and its manifest pipeline copy, list no resources when
// there are more than the custom resource quota.
func TestCRCollectorQuota(t *testing.T) {
	crCollector := newTestCRCollector(t)
	crCollector.maxCount = 1
	for _, collector := range []*ResourceCollector{crCollector, crCollector.ForManifests()} {
		runCfg, start := registry.NewTestRunConfig(t, []runtime.Object{testCustomResource("a"), testCustomResource("b")})
		collector.Init(runCfg)
		start()
		require.True(t, cache.WaitForCacheSync(runCfg.StopCh, collector.Informer().HasSynced))

		result, err := collector.Run(runCfg)
		assert.Nil(t, result)
		assert.ErrorContains(t, err, "has reached to the limit 1")
	}
}

// TestResourceCollectorFailsClosed checks that a collector whose informer cannot be created sends nothing.
func TestResourceCollectorFailsClosed(t *testing.T) {
	failure := errors.New("transform not set")
	resource := registry.Resources()[0]
	resource.Informer = func(*collectors.CollectorRunConfig, schema.GroupVersionResource, func(any)) (cache.SharedIndexInformer, error) {
		return nil, failure
	}
	for _, manifestsOnly := range []bool{false, true} {
		collector := NewResourceCollector(resource, resource.Versions[0], registry.Dependencies{Tagger: processorstest.NewEmptyFakeTagger()}, manifestsOnly)
		runCfg, _ := registry.NewTestRunConfig(t, nil)
		collector.Init(runCfg)

		result, err := collector.Run(runCfg)
		assert.ErrorIs(t, err, failure)
		assert.Nil(t, result)

		result, err = collector.Process(runCfg, []interface{}{registry.ResourceFixtures()[collector.Metadata().FullName()].Object})
		assert.ErrorIs(t, err, failure)
		assert.Nil(t, result)
	}
}

// newTestCRCollector returns a custom resource collector with the kind that discovery sets.
func newTestCRCollector(t *testing.T) *ResourceCollector {
	collector, err := NewCRCollector("datadogmetrics", "datadoghq.com/v1alpha1")
	require.NoError(t, err)
	collector.Metadata().Kind = "DatadogMetric"
	return collector
}

func testCustomResource(name string) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "datadoghq.com/v1alpha1",
		"kind":       "DatadogMetric",
		"metadata":   map[string]interface{}{"name": name, "namespace": "default", "uid": "uid-" + name, "resourceVersion": "1"},
		"spec":       map[string]interface{}{"query": "avg:system.cpu.user{*}"},
	}}
}

// assertResourceResult checks the metadata and the manifest that a collector built for its fixture.
func assertResourceResult(t *testing.T, collector *ResourceCollector, fixture registry.ResourceFixture, result *collectors.CollectorRunResult, hasMetadata, manifestsOnly, terminated bool) {
	metadata := collector.Metadata()

	if hasMetadata {
		require.Len(t, result.Result.MetadataMessages, 1)
		assert.IsType(t, fixture.MetadataType, result.Result.MetadataMessages[0])
	} else {
		// A manifest-only resource yields nil metadata bodies, which are never sent.
		for _, message := range result.Result.MetadataMessages {
			assert.Nil(t, message)
		}
	}

	require.Len(t, result.Result.ManifestMessages, 1)
	var manifests []*model.Manifest
	switch message := result.Result.ManifestMessages[0].(type) {
	case *model.CollectorManifest:
		assert.False(t, !manifestsOnly && (metadata.NodeType == orchestrator.K8sCRD || metadata.NodeType == orchestrator.K8sCR), "the combined pipeline sends CRs and CRDs in their envelope")
		manifests = message.Manifests
	case *model.CollectorManifestCRD:
		assert.True(t, !manifestsOnly && metadata.NodeType == orchestrator.K8sCRD, "only the combined pipeline sends the CRD envelope")
		manifests = message.Manifest.Manifests
	case *model.CollectorManifestCR:
		assert.True(t, !manifestsOnly && metadata.NodeType == orchestrator.K8sCR, "only the combined pipeline sends the CR envelope")
		manifests = message.Manifest.Manifests
	default:
		t.Fatalf("unexpected manifest payload type %T", message)
	}

	require.Len(t, manifests, 1)
	manifest := manifests[0]
	assert.Equal(t, string(fixture.Object.(metav1.Object).GetUID()), manifest.Uid)
	assert.Equal(t, int32(metadata.NodeType), manifest.Type)
	assert.Equal(t, metadata.Kind, manifest.Kind)
	assert.Equal(t, metadata.GroupVersion(), manifest.ApiVersion)
	assert.Equal(t, terminated, manifest.IsTerminated)
	assert.Contains(t, manifest.Tags, "kube_api_version:"+metadata.GroupVersion())

	var content map[string]interface{}
	require.NoError(t, json.Unmarshal(manifest.Content, &content))
	assert.Equal(t, metadata.Kind, content["kind"])
	assert.Equal(t, fixture.Object.(metav1.Object).GetName(), content["metadata"].(map[string]interface{})["name"])
	// Metadata-only informers store no data, while Helm releases are sent with theirs.
	switch fixture.Object.(type) {
	case *metav1.PartialObjectMetadata:
		assert.NotContains(t, content, "data")
	case *corev1.ConfigMap:
		assert.Contains(t, content, "data")
	}
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
