// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package infraattributesprocessor

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processortest"
	"go.uber.org/zap"

	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/otelcol/otlp/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// countingTagger wraps testutil.TestTaggerClient and records how often each
// tagger method is called, so that tests can assert on the per-batch
// memoization itself rather than only on the attributes it produces.
//
// It also honors cardinality, which TestTaggerClient deliberately ignores --
// without that, a regression that queried the tagger at the wrong cardinality
// would stay invisible.
type countingTagger struct {
	*testutil.TestTaggerClient

	// tagErrors makes Tag fail for the listed entities.
	tagErrors map[types.EntityID]error
	// highCardTags is returned in addition to TagMap for HighCardinality.
	highCardTags map[types.EntityID][]string

	mu               sync.Mutex
	tagCalls         map[tagCacheKey]int
	globalTagsCalls  map[types.TagCardinality]int
	containerIDCalls int
}

var _ types.TaggerClient = (*countingTagger)(nil)

func newCountingTagger() *countingTagger {
	return &countingTagger{
		TestTaggerClient: testutil.NewTestTaggerClient(),
		tagErrors:        map[types.EntityID]error{},
		highCardTags:     map[types.EntityID][]string{},
		tagCalls:         map[tagCacheKey]int{},
		globalTagsCalls:  map[types.TagCardinality]int{},
	}
}

func (c *countingTagger) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	c.mu.Lock()
	c.tagCalls[tagCacheKey{entityID: entityID, cardinality: cardinality}]++
	c.mu.Unlock()
	if err, ok := c.tagErrors[entityID]; ok {
		return nil, err
	}
	entityTags, err := c.TestTaggerClient.Tag(entityID, cardinality)
	if err != nil || cardinality != types.HighCardinality {
		return entityTags, err
	}
	return append(append([]string{}, entityTags...), c.highCardTags[entityID]...), nil
}

func (c *countingTagger) GlobalTags(cardinality types.TagCardinality) ([]string, error) {
	c.mu.Lock()
	c.globalTagsCalls[cardinality]++
	c.mu.Unlock()
	return c.TestTaggerClient.GlobalTags(cardinality)
}

func (c *countingTagger) GenerateContainerIDFromOriginInfo(originInfo origindetection.OriginInfo) (string, error) {
	c.mu.Lock()
	c.containerIDCalls++
	c.mu.Unlock()
	return c.TestTaggerClient.GenerateContainerIDFromOriginInfo(originInfo)
}

// tagCallsFor returns the number of Tag calls made for entityID at any cardinality.
func (c *countingTagger) tagCallsFor(entityID types.EntityID) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for key, calls := range c.tagCalls {
		if key.entityID == entityID {
			n += calls
		}
	}
	return n
}

func (c *countingTagger) totalTagCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, calls := range c.tagCalls {
		n += calls
	}
	return n
}

func (c *countingTagger) totalGlobalTagsCalls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, calls := range c.globalTagsCalls {
		n += calls
	}
	return n
}

// The entities a resource carrying batchResourceAttributes resolves to, in the
// order entityIDsFromAttributes emits them.
var (
	containerEntity  = types.NewEntityID(types.ContainerID, "cid-1")
	deploymentEntity = types.NewEntityID(types.KubernetesDeployment, "ns/dep")
	namespaceEntity  = types.NewEntityID(types.KubernetesMetadata, "/namespaces//ns")
	nodeEntity       = types.NewEntityID(types.KubernetesNode, "node-1")
	podEntity        = types.NewEntityID(types.KubernetesPodUID, "pod-1")
	globalEntity     = types.NewEntityID("internal", "global-entity-id")
)

// batchResourceAttributes resolves to five entities. container.id is set so
// that GenerateContainerIDFromOriginInfo is not consulted, keeping the call
// counts in these tests attributable to Tag/GlobalTags alone.
var batchResourceAttributes = map[string]any{
	"container.id":        "cid-1",
	"k8s.pod.uid":         "pod-1",
	"k8s.namespace.name":  "ns",
	"k8s.deployment.name": "dep",
	"k8s.node.name":       "node-1",
}

// batchResourceWantAttributes is batchResourceAttributes plus every tag the
// fixture tagger hands out. kube_namespace is returned by two entities, which
// pins the deduplication behavior.
var batchResourceWantAttributes = map[string]any{
	"container.id":        "cid-1",
	"k8s.pod.uid":         "pod-1",
	"k8s.namespace.name":  "ns",
	"k8s.deployment.name": "dep",
	"k8s.node.name":       "node-1",

	"container_name":  "nginx",
	"image_tag":       "1.0",
	"kube_namespace":  "ns",
	"pod_phase":       "running",
	"kube_deployment": "dep",
	"kube_node":       "node-1",
	"global":          "tag",
}

// newBatchFixtureTagger returns a tagger populated for batchResourceAttributes.
func newBatchFixtureTagger() *countingTagger {
	tc := newCountingTagger()
	tc.TagMap[containerEntity.String()] = []string{"container_name:nginx", "image_tag:1.0"}
	tc.TagMap[podEntity.String()] = []string{"kube_namespace:ns", "pod_phase:running"}
	tc.TagMap[namespaceEntity.String()] = []string{"kube_namespace:ns"}
	tc.TagMap[deploymentEntity.String()] = []string{"kube_deployment:dep"}
	tc.TagMap[nodeEntity.String()] = []string{"kube_node:node-1"}
	tc.TagMap[globalEntity.String()] = []string{"global:tag"}
	return tc
}

// newTestMetricsProcessor builds the metrics processor over tc.
func newTestMetricsProcessor(t testing.TB, tc types.TaggerClient, cfg *Config, next *consumertest.MetricsSink) processor.Metrics {
	t.Helper()
	factory := NewFactoryForAgent(tc, func(_ context.Context) (string, error) {
		return "test-host", nil
	})
	proc, err := factory.CreateMetrics(context.Background(), processortest.NewNopSettings(Type), cfg, next)
	require.NoError(t, err)
	require.NoError(t, proc.Start(context.Background(), nil))
	t.Cleanup(func() { assert.NoError(t, proc.Shutdown(context.Background())) })
	return proc
}

func repeatResource(attrs map[string]any, n int) []metricWithResource {
	mwrs := make([]metricWithResource, 0, n)
	for i := 0; i < n; i++ {
		mwrs = append(mwrs, metricWithResource{metricNames: inMetricNames, resourceAttributes: attrs})
	}
	return mwrs
}

// TestTagBatchQueriesTaggerOncePerBatch is the core assertion of the per-batch
// cache: a batch of identical resources costs one Tag call per distinct entity
// and one GlobalTags call, not one of each per resource -- and every resource
// still comes out fully tagged.
func TestTagBatchQueriesTaggerOncePerBatch(t *testing.T) {
	const resources = 4
	tc := newBatchFixtureTagger()
	next := new(consumertest.MetricsSink)
	proc := newTestMetricsProcessor(t, tc, &Config{Cardinality: types.LowCardinality}, next)

	require.NoError(t, proc.ConsumeMetrics(context.Background(), testResourceMetrics(repeatResource(batchResourceAttributes, resources))))

	require.Len(t, next.AllMetrics(), 1)
	rms := next.AllMetrics()[0].ResourceMetrics()
	require.Equal(t, resources, rms.Len())
	for i := 0; i < rms.Len(); i++ {
		assert.EqualValues(t, batchResourceWantAttributes, rms.At(i).Resource().Attributes().AsRaw(), "resource %d", i)
	}

	for _, entityID := range []types.EntityID{containerEntity, deploymentEntity, namespaceEntity, nodeEntity, podEntity} {
		assert.Equal(t, 1, tc.tagCallsFor(entityID), "Tag calls for %s", entityID)
	}
	assert.Equal(t, 5, tc.totalTagCalls(), "one Tag call per distinct entity, not per resource")
	assert.Equal(t, 1, tc.totalGlobalTagsCalls(), "one GlobalTags call per batch, not per resource")
}

// TestTagBatchUsesConfiguredCardinality guards against the cache being keyed
// or queried at the wrong cardinality.
func TestTagBatchUsesConfiguredCardinality(t *testing.T) {
	tc := newBatchFixtureTagger()
	tc.highCardTags[containerEntity] = []string{"container_id_short:cid"}
	next := new(consumertest.MetricsSink)
	proc := newTestMetricsProcessor(t, tc, &Config{Cardinality: types.HighCardinality}, next)

	require.NoError(t, proc.ConsumeMetrics(context.Background(), testResourceMetrics(repeatResource(batchResourceAttributes, 3))))

	want := map[string]any{"container_id_short": "cid"}
	for k, v := range batchResourceWantAttributes {
		want[k] = v
	}
	require.Len(t, next.AllMetrics(), 1)
	rms := next.AllMetrics()[0].ResourceMetrics()
	for i := 0; i < rms.Len(); i++ {
		assert.EqualValues(t, want, rms.At(i).Resource().Attributes().AsRaw(), "resource %d", i)
	}

	tc.mu.Lock()
	defer tc.mu.Unlock()
	assert.Equal(t, 1, tc.tagCalls[tagCacheKey{entityID: containerEntity, cardinality: types.HighCardinality}])
	assert.Equal(t, 1, tc.globalTagsCalls[types.HighCardinality])
	assert.Len(t, tc.globalTagsCalls, 1)
}

// TestTagBatchNoCrossContamination pins that resources sharing a batch do not
// inherit each other's tags.
func TestTagBatchNoCrossContamination(t *testing.T) {
	tc := newBatchFixtureTagger()
	next := new(consumertest.MetricsSink)
	proc := newTestMetricsProcessor(t, tc, &Config{Cardinality: types.LowCardinality}, next)

	md := testResourceMetrics([]metricWithResource{
		{metricNames: inMetricNames, resourceAttributes: map[string]any{"container.id": "cid-1"}},
		{metricNames: inMetricNames, resourceAttributes: map[string]any{"k8s.node.name": "node-1"}},
		// Back to the first entity: it must come from the cache and still be correct.
		{metricNames: inMetricNames, resourceAttributes: map[string]any{"container.id": "cid-1"}},
	})
	require.NoError(t, proc.ConsumeMetrics(context.Background(), md))

	wantContainer := map[string]any{
		"container.id":   "cid-1",
		"container_name": "nginx",
		"image_tag":      "1.0",
		"global":         "tag",
	}
	wantNode := map[string]any{
		"k8s.node.name": "node-1",
		"kube_node":     "node-1",
		"global":        "tag",
		// container.id is derived for resources that lack it; the fixture
		// tagger cannot resolve one here, so none is added.
	}

	require.Len(t, next.AllMetrics(), 1)
	rms := next.AllMetrics()[0].ResourceMetrics()
	require.Equal(t, 3, rms.Len())
	assert.EqualValues(t, wantContainer, rms.At(0).Resource().Attributes().AsRaw())
	assert.EqualValues(t, wantNode, rms.At(1).Resource().Attributes().AsRaw())
	assert.EqualValues(t, wantContainer, rms.At(2).Resource().Attributes().AsRaw())

	assert.Equal(t, 1, tc.tagCallsFor(containerEntity))
	assert.Equal(t, 1, tc.tagCallsFor(nodeEntity))
	assert.Equal(t, 2, tc.totalTagCalls())
	assert.Equal(t, 1, tc.totalGlobalTagsCalls())
}

// TestTagBatchDoesNotOutliveBatch is the freshness guarantee: the cache is
// scoped to one batch, so a tagger update is picked up by the next one.
func TestTagBatchDoesNotOutliveBatch(t *testing.T) {
	tc := newBatchFixtureTagger()
	next := new(consumertest.MetricsSink)
	proc := newTestMetricsProcessor(t, tc, &Config{Cardinality: types.LowCardinality}, next)

	require.NoError(t, proc.ConsumeMetrics(context.Background(), testResourceMetrics(repeatResource(batchResourceAttributes, 2))))
	assert.Equal(t, 5, tc.totalTagCalls())
	assert.Equal(t, 1, tc.totalGlobalTagsCalls())

	// A tagger update between batches must be visible to the second batch.
	tc.TagMap[nodeEntity.String()] = []string{"kube_node:node-2"}
	tc.TagMap[globalEntity.String()] = []string{"global:updated"}

	require.NoError(t, proc.ConsumeMetrics(context.Background(), testResourceMetrics(repeatResource(batchResourceAttributes, 2))))
	assert.Equal(t, 10, tc.totalTagCalls(), "the second batch re-queries the tagger")
	assert.Equal(t, 2, tc.totalGlobalTagsCalls())

	want := map[string]any{}
	for k, v := range batchResourceWantAttributes {
		want[k] = v
	}
	want["kube_node"] = "node-2"
	want["global"] = "updated"

	require.Len(t, next.AllMetrics(), 2)
	rms := next.AllMetrics()[1].ResourceMetrics()
	require.Equal(t, 2, rms.Len())
	for i := 0; i < rms.Len(); i++ {
		assert.EqualValues(t, want, rms.At(i).Resource().Attributes().AsRaw(), "resource %d", i)
	}
}

// TestTagBatchMemoizesLookupFailures pins that a failing entity is queried --
// and therefore logged -- once per batch, and that the failure does not cost
// the resource its other tags.
func TestTagBatchMemoizesLookupFailures(t *testing.T) {
	tc := newBatchFixtureTagger()
	tc.tagErrors[containerEntity] = errors.New("entity not found")
	next := new(consumertest.MetricsSink)
	proc := newTestMetricsProcessor(t, tc, &Config{Cardinality: types.LowCardinality}, next)

	require.NoError(t, proc.ConsumeMetrics(context.Background(), testResourceMetrics(repeatResource(batchResourceAttributes, 3))))

	want := map[string]any{}
	for k, v := range batchResourceWantAttributes {
		want[k] = v
	}
	delete(want, "container_name")
	delete(want, "image_tag")

	require.Len(t, next.AllMetrics(), 1)
	rms := next.AllMetrics()[0].ResourceMetrics()
	require.Equal(t, 3, rms.Len())
	for i := 0; i < rms.Len(); i++ {
		assert.EqualValues(t, want, rms.At(i).Resource().Attributes().AsRaw(), "resource %d", i)
	}
	assert.Equal(t, 1, tc.tagCallsFor(containerEntity), "a failing entity is queried once per batch")
}

// TestTagBatchConcurrentBatches runs concurrent batches through one processor.
// Meaningful under -race: the cache must be per-call state, never shared.
func TestTagBatchConcurrentBatches(t *testing.T) {
	tc := newBatchFixtureTagger()
	next := new(consumertest.MetricsSink)
	proc := newTestMetricsProcessor(t, tc, &Config{Cardinality: types.LowCardinality}, next)

	const goroutines = 8
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, proc.ConsumeMetrics(context.Background(), testResourceMetrics(repeatResource(batchResourceAttributes, 3))))
		}()
	}
	wg.Wait()

	require.Len(t, next.AllMetrics(), goroutines)
	for _, md := range next.AllMetrics() {
		rms := md.ResourceMetrics()
		require.Equal(t, 3, rms.Len())
		for i := 0; i < rms.Len(); i++ {
			assert.EqualValues(t, batchResourceWantAttributes, rms.At(i).Resource().Attributes().AsRaw())
		}
	}
	// Each batch has its own cache, so the tagger sees one round of lookups
	// per batch and never more than that.
	assert.Equal(t, 5*goroutines, tc.totalTagCalls())
	assert.Equal(t, goroutines, tc.totalGlobalTagsCalls())
}

// TestTagBatchCachesPerCardinality exercises the cache directly: a batch
// processed at two cardinalities must query the tagger once for each.
func TestTagBatchCachesPerCardinality(t *testing.T) {
	tc := newBatchFixtureTagger()
	batch := newInfraTagsProcessor(tc, option.None[SourceProviderFunc]()).newTagBatch()
	logger := zap.NewNop()

	low := batch.tagsFor(logger, containerEntity, types.LowCardinality)
	assert.Equal(t, []string{"container_name:nginx", "image_tag:1.0"}, low)
	assert.Equal(t, low, batch.tagsFor(logger, containerEntity, types.LowCardinality))
	assert.Equal(t, 1, tc.tagCallsFor(containerEntity))

	tc.highCardTags[containerEntity] = []string{"container_id_short:cid"}
	high := batch.tagsFor(logger, containerEntity, types.HighCardinality)
	assert.Equal(t, []string{"container_name:nginx", "image_tag:1.0", "container_id_short:cid"}, high)
	assert.Equal(t, 2, tc.tagCallsFor(containerEntity))

	assert.Equal(t, []string{"global:tag"}, batch.globalTags(logger, types.LowCardinality))
	assert.Equal(t, []string{"global:tag"}, batch.globalTags(logger, types.LowCardinality))
	assert.Equal(t, 1, tc.totalGlobalTagsCalls())
	assert.Equal(t, []string{"global:tag"}, batch.globalTags(logger, types.HighCardinality))
	assert.Equal(t, 2, tc.totalGlobalTagsCalls())
}

func TestSplitTag(t *testing.T) {
	tests := []struct {
		tag       string
		wantKey   string
		wantValue string
	}{
		{tag: "key:value", wantKey: "key", wantValue: "value"},
		{tag: "key:value:with:colons", wantKey: "key", wantValue: "value:with:colons"},
		{tag: "key:", wantKey: "", wantValue: ""},
		{tag: ":value", wantKey: "", wantValue: ""},
		{tag: ":", wantKey: "", wantValue: ""},
		{tag: "novalue", wantKey: "", wantValue: ""},
		{tag: "", wantKey: "", wantValue: ""},
	}
	for _, test := range tests {
		t.Run(test.tag, func(t *testing.T) {
			key, value := splitTag(test.tag)
			assert.Equal(t, test.wantKey, key)
			assert.Equal(t, test.wantValue, value)
		})
	}
}
