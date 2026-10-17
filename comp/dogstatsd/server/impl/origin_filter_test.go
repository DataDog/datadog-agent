// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build cel && test

package serverimpl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/origindetection"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	noopTelemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/impl/noops"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadfilterimpl "github.com/DataDog/datadog-agent/comp/core/workloadfilter/impl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
	metricsmock "github.com/DataDog/datadog-agent/pkg/util/containers/metrics/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/util/metricname"
)

const testDogstatsdRules = `
cel_workload_exclude:
- products: ["dogstatsd"]
  rules:
    containers:
      - "container.pod.namespace == 'noisy'"
      - "container.image.reference.startsWith('busybox')"
`

// countingMetaCollector counts the container ID resolutions it serves.
type countingMetaCollector struct {
	metricsmock.MetaCollector
	inodeCalls        int
	externalDataCalls int
}

func (c *countingMetaCollector) GetContainerIDForInode(inode uint64, cacheValidity time.Duration) (string, error) {
	c.inodeCalls++
	return c.MetaCollector.GetContainerIDForInode(inode, cacheValidity)
}

func (c *countingMetaCollector) ContainerIDForPodUIDAndContName(podUID, contName string, initCont bool, cacheValidity time.Duration) (string, error) {
	c.externalDataCalls++
	return c.MetaCollector.ContainerIDForPodUIDAndContName(podUID, contName, initCont, cacheValidity)
}

type originFilterFixture struct {
	wmeta         workloadmetamock.Mock
	metaCollector *countingMetaCollector
	filterStore   workloadfilter.Component
}

func newOriginFilterFixture(t *testing.T, yamlConfig string) *originFilterFixture {
	cfg := configmock.NewFromYAML(t, yamlConfig)
	provides, err := workloadfilterimpl.NewComponent(workloadfilterimpl.Requires{
		Log:       logmock.New(t),
		Config:    cfg,
		Telemetry: noopTelemetry.GetCompatComponent(),
	})
	require.NoError(t, err)

	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Provide(func() config.Component { return cfg }),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	// Pod "noisy-pod" in the "noisy" namespace with container "noisy-cid",
	// pod "default-pod" in the "default" namespace with container "default-cid",
	// and container "busybox-cid" without a pod.
	for _, pod := range []struct{ id, namespace, containerID, containerName string }{
		{"noisy-pod", "noisy", "noisy-cid", "app"},
		{"default-pod", "default", "default-cid", "app"},
	} {
		wmeta.Set(&workloadmeta.KubernetesPod{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: pod.id},
			EntityMeta: workloadmeta.EntityMeta{Name: pod.id, Namespace: pod.namespace},
			Containers: []workloadmeta.OrchestratorContainer{{ID: pod.containerID, Name: pod.containerName}},
		})
		wmeta.Set(&workloadmeta.Container{
			EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: pod.containerID},
			EntityMeta: workloadmeta.EntityMeta{Name: pod.containerName},
			Owner:      &workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: pod.id},
		})
	}
	wmeta.Set(&workloadmeta.Container{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "busybox-cid"},
		EntityMeta: workloadmeta.EntityMeta{Name: "box"},
		Image:      workloadmeta.ContainerImage{RawName: "busybox:latest"},
	})

	return &originFilterFixture{
		wmeta: wmeta,
		metaCollector: &countingMetaCollector{MetaCollector: metricsmock.MetaCollector{
			CIDFromInode:          map[uint64]string{1: "noisy-cid", 2: "default-cid"},
			CIDFromPodUIDContName: map[string]string{"noisy-pod/app": "noisy-cid", "default-pod/app": "default-cid"},
		}},
		filterStore: provides.Comp,
	}
}

func (f *originFilterFixture) newFilter(dropUnresolved bool) *originFilter {
	return newOriginFilter(f.wmeta, f.filterStore, f.metaCollector, dropUnresolved)
}

func udsOrigin(containerID string) string {
	return types.NewEntityID(types.ContainerID, containerID).String()
}

func TestOriginFilterShouldDrop(t *testing.T) {
	fixture := newOriginFilterFixture(t, testDogstatsdRules)

	for _, tt := range []struct {
		name           string
		origin         taggertypes.OriginInfo
		dropUnresolved bool
		expected       bool
	}{
		{
			name:     "excluded by pod namespace via UDS origin",
			origin:   taggertypes.OriginInfo{ContainerIDFromSocket: udsOrigin("noisy-cid")},
			expected: true,
		},
		{
			name:     "excluded by image via client container ID",
			origin:   taggertypes.OriginInfo{LocalData: origindetection.LocalData{ContainerID: "busybox-cid"}},
			expected: true,
		},
		{
			name:     "excluded via inode",
			origin:   taggertypes.OriginInfo{LocalData: origindetection.LocalData{Inode: 1}},
			expected: true,
		},
		{
			name:     "excluded via external data",
			origin:   taggertypes.OriginInfo{ExternalData: origindetection.ExternalData{PodUID: "noisy-pod", ContainerName: "app"}},
			expected: true,
		},
		{
			name:     "not matching container is kept",
			origin:   taggertypes.OriginInfo{ContainerIDFromSocket: udsOrigin("default-cid")},
			expected: false,
		},
		{
			name: "UDS origin has precedence over the client container ID",
			origin: taggertypes.OriginInfo{
				ContainerIDFromSocket: udsOrigin("default-cid"),
				LocalData:             origindetection.LocalData{ContainerID: "noisy-cid"},
			},
			expected: false,
		},
		{
			name:     "container unknown to workloadmeta is kept",
			origin:   taggertypes.OriginInfo{LocalData: origindetection.LocalData{ContainerID: "unknown-cid"}},
			expected: false,
		},
		{
			name:     "pod-only origin is unresolved and kept",
			origin:   taggertypes.OriginInfo{LocalData: origindetection.LocalData{PodUID: "noisy-pod"}},
			expected: false,
		},
		{
			name:     "unresolved origin is kept by default",
			origin:   taggertypes.OriginInfo{},
			expected: false,
		},
		{
			name:           "unresolved origin is dropped when configured",
			origin:         taggertypes.OriginInfo{},
			dropUnresolved: true,
			expected:       true,
		},
		{
			name:           "unresolvable inode is dropped when configured",
			origin:         taggertypes.OriginInfo{LocalData: origindetection.LocalData{Inode: 42}},
			dropUnresolved: true,
			expected:       true,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			origin := tt.origin
			assert.Equal(t, tt.expected, fixture.newFilter(tt.dropUnresolved).shouldDrop(&origin))
		})
	}
}

func TestOriginFilterHandsResolutionToTagger(t *testing.T) {
	fixture := newOriginFilterFixture(t, testDogstatsdRules)
	filter := fixture.newFilter(false)

	t.Run("free resolutions are not handed off", func(t *testing.T) {
		origin := taggertypes.OriginInfo{ContainerIDFromSocket: udsOrigin("default-cid"), LocalData: origindetection.LocalData{Inode: 1}}
		filter.shouldDrop(&origin)
		assert.Nil(t, origin.Resolved)
		assert.Zero(t, fixture.metaCollector.inodeCalls)
	})

	t.Run("inode resolution is handed off", func(t *testing.T) {
		origin := taggertypes.OriginInfo{LocalData: origindetection.LocalData{Inode: 2}}
		filter.shouldDrop(&origin)
		require.NotNil(t, origin.Resolved)
		assert.Equal(t, taggertypes.ResolvedOrigin{InodeContainerID: "default-cid", InodeDone: true}, *origin.Resolved)
	})

	t.Run("external data is only resolved when the inode is not enough", func(t *testing.T) {
		origin := taggertypes.OriginInfo{
			LocalData:    origindetection.LocalData{Inode: 42},
			ExternalData: origindetection.ExternalData{PodUID: "default-pod", ContainerName: "app"},
		}
		filter.shouldDrop(&origin)
		require.NotNil(t, origin.Resolved)
		assert.Equal(t, taggertypes.ResolvedOrigin{InodeDone: true, ExternalDataContainerID: "default-cid", ExternalDataDone: true}, *origin.Resolved)
	})
}

func TestOriginFilterCaches(t *testing.T) {
	fixture := newOriginFilterFixture(t, testDogstatsdRules)
	filter := fixture.newFilter(false)

	origin := func() *taggertypes.OriginInfo {
		return &taggertypes.OriginInfo{ExternalData: origindetection.ExternalData{PodUID: "noisy-pod", ContainerName: "app"}}
	}

	assert.True(t, filter.shouldDrop(origin()))
	assert.True(t, filter.shouldDrop(origin()))
	assert.Equal(t, 1, fixture.metaCollector.externalDataCalls, "resolution should be cached")

	// The resolution cache expires quickly so that container restarts are picked up
	filter.now = filter.now.Add(resolutionTTL + time.Millisecond)
	assert.True(t, filter.shouldDrop(origin()))
	assert.Equal(t, 2, fixture.metaCollector.externalDataCalls)
}

func TestOriginFilterNewContainer(t *testing.T) {
	fixture := newOriginFilterFixture(t, testDogstatsdRules)
	filter := fixture.newFilter(false)

	origin := func() *taggertypes.OriginInfo {
		return &taggertypes.OriginInfo{ContainerIDFromSocket: udsOrigin("new-cid")}
	}

	// Not known by workloadmeta yet: kept, and not cached
	assert.False(t, filter.shouldDrop(origin()))
	assert.NotContains(t, filter.decisions, "new-cid")

	fixture.wmeta.Set(&workloadmeta.Container{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "new-cid"},
		EntityMeta: workloadmeta.EntityMeta{Name: "new"},
		Image:      workloadmeta.ContainerImage{RawName: "busybox:1.36"},
	})

	// Decided on the next sample once the container is known
	assert.True(t, filter.shouldDrop(origin()))
}

func TestOriginFilterContainerNotLinkedToPodYet(t *testing.T) {
	fixture := newOriginFilterFixture(t, testDogstatsdRules)
	filter := fixture.newFilter(false)

	origin := func() *taggertypes.OriginInfo {
		return &taggertypes.OriginInfo{ContainerIDFromSocket: udsOrigin("late-cid")}
	}

	// The runtime knows the Kubernetes container before the kubelet links it to its pod
	labels := map[string]string{kubernetes.CriContainerNamespaceLabel: "noisy"}
	fixture.wmeta.Set(&workloadmeta.Container{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "late-cid"},
		EntityMeta: workloadmeta.EntityMeta{Name: "app", Labels: labels},
	})
	assert.False(t, filter.shouldDrop(origin()))
	assert.NotContains(t, filter.decisions, "late-cid")

	fixture.wmeta.Set(&workloadmeta.Container{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "late-cid"},
		EntityMeta: workloadmeta.EntityMeta{Name: "app", Labels: labels},
		Owner:      &workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "noisy-pod"},
	})

	// Decided on the next sample once the pod is linked
	assert.True(t, filter.shouldDrop(origin()))
}

func TestOriginFilterCachesCompleteDecisions(t *testing.T) {
	fixture := newOriginFilterFixture(t, testDogstatsdRules)
	filter := fixture.newFilter(false)

	origin := func() *taggertypes.OriginInfo {
		return &taggertypes.OriginInfo{ContainerIDFromSocket: udsOrigin("busybox-cid")}
	}

	// A container without a pod (e.g. Docker) has complete metadata: the decision is cached
	assert.True(t, filter.shouldDrop(origin()))
	assert.Contains(t, filter.decisions, "busybox-cid")

	fixture.wmeta.Set(&workloadmeta.Container{
		EntityID:   workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "busybox-cid"},
		EntityMeta: workloadmeta.EntityMeta{Name: "box"},
		Image:      workloadmeta.ContainerImage{RawName: "alpine:latest"},
	})
	assert.True(t, filter.shouldDrop(origin()), "decision should still be cached")

	filter.now = filter.now.Add(decisionTTL + time.Millisecond)
	assert.False(t, filter.shouldDrop(origin()), "decision should be re-evaluated after the TTL")
}

func TestWorkloadFilterEnabled(t *testing.T) {
	for _, tt := range []struct {
		name     string
		yaml     string
		expected bool
	}{
		{name: "no configuration", yaml: "", expected: false},
		{name: "dogstatsd rules", yaml: testDogstatsdRules, expected: true},
		{
			name: "rules for other products only",
			yaml: `
cel_workload_exclude:
- products: ["metrics", "global"]
  rules:
    containers: ["container.name == 'foo'"]
`,
			expected: false,
		},
		{name: "drop unresolved", yaml: "dogstatsd_workload_filter_drop_unresolved: true", expected: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, workloadFilterEnabled(configmock.NewFromYAML(t, tt.yaml)))
		})
	}
}

func TestParseMessagesWithOriginFilter(t *testing.T) {
	fixture := newOriginFilterFixture(t, testDogstatsdRules)
	deps, s := fulfillDepsWithInactiveServer(t, map[string]interface{}{})

	parser := newParser(deps.Config, s.sharedFloat64List, 1, deps.WMeta, s.stringInternerTelemetry)
	parser.dsdOriginEnabled = true
	parser.originFilter = fixture.newFilter(false)

	// Excluded origin
	samples, err := s.parseMetricMessage(nil, parser, []byte("metric.name:1:2:3|g|c:noisy-cid"), "", 0, "", false, nil)
	assert.NoError(t, err)
	assert.Empty(t, samples)

	event, err := s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:noisy-cid"), "", 0)
	assert.NoError(t, err)
	assert.Nil(t, event)

	serviceCheck, err := s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:noisy-cid"), "", 0)
	assert.NoError(t, err)
	assert.Nil(t, serviceCheck)

	// Kept origin
	samples, err = s.parseMetricMessage(nil, parser, []byte("metric.name:1:2:3|g|c:default-cid"), "", 0, "", false, nil)
	assert.NoError(t, err)
	assert.Len(t, samples, 3)

	event, err = s.parseEventMessage(parser, []byte("_e{10,10}:event title|test\\ntext|c:default-cid"), "", 0)
	assert.NoError(t, err)
	assert.NotNil(t, event)

	serviceCheck, err = s.parseServiceCheckMessage(parser, []byte("_sc|service-check.name|0|c:default-cid"), "", 0)
	assert.NoError(t, err)
	assert.NotNil(t, serviceCheck)

	// Multi-value messages share the resolution handed to the tagger
	samples, err = s.parseMetricMessage(nil, parser, []byte("metric.name:1:2|g|c:in-2"), "", 0, "", false, nil)
	assert.NoError(t, err)
	require.Len(t, samples, 2)
	require.NotNil(t, samples[0].OriginInfo.Resolved)
	assert.Equal(t, "default-cid", samples[0].OriginInfo.Resolved.InodeContainerID)
	assert.Same(t, samples[0].OriginInfo.Resolved, samples[1].OriginInfo.Resolved)

	// Inode resolving to an excluded container
	samples, err = s.parseMetricMessage(nil, parser, []byte("metric.name:1|g|c:in-1"), "", 0, "", false, nil)
	assert.NoError(t, err)
	assert.Empty(t, samples)
}

func TestOriginFilterNotBuiltByDefault(t *testing.T) {
	_, s := fulfillDepsWithInactiveServer(t, map[string]interface{}{})
	assert.False(t, s.originFilterEnabled)

	w := newWorker(s, 0, s.wmeta, s.packetsTelemetry, s.stringInternerTelemetry, metricname.Matcher{})
	assert.Nil(t, w.parser.originFilter)
}
