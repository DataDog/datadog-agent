// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build anomalydetection_recorder

package recorderimpl

import (
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	recorder "github.com/DataDog/datadog-agent/comp/anomalydetection/recorder/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/stretchr/testify/require"
)

type metricFilterWMeta struct {
	workloadmeta.Component
	container *workloadmeta.Container
	pod       *workloadmeta.KubernetesPod
}

func (w *metricFilterWMeta) GetContainer(id string) (*workloadmeta.Container, error) {
	if w.container != nil && w.container.EntityID.ID == id {
		return w.container, nil
	}
	return nil, nil
}

func (w *metricFilterWMeta) GetKubernetesPodByName(_ string, _ string) (*workloadmeta.KubernetesPod, error) {
	return w.pod, nil
}

func (w *metricFilterWMeta) GetKubernetesPodForContainer(_ string) (*workloadmeta.KubernetesPod, error) {
	return w.pod, nil
}

type metricFilterStore struct {
	workloadfilter.Component
	bundle workloadfilter.FilterBundle
}

func (s *metricFilterStore) GetContainerAutodiscoveryFilters(_ workloadfilter.Scope) workloadfilter.FilterBundle {
	return s.bundle
}

type metricFilterBundle struct {
	excluded bool
}

func (b *metricFilterBundle) IsExcluded(_ workloadfilter.Filterable) bool { return b.excluded }
func (b *metricFilterBundle) GetResult(_ workloadfilter.Filterable) workloadfilter.Result {
	if b.excluded {
		return workloadfilter.Excluded
	}
	return workloadfilter.Included
}
func (*metricFilterBundle) GetErrors() []error { return nil }

type stubMetricFilter struct {
	excluded bool
}

func (f *stubMetricFilter) IsExcluded(_ observer.MetricView) bool { return f.excluded }

func newMetricFilter(t *testing.T, wmeta *metricFilterWMeta, excluded bool) recorder.MetricFilter {
	t.Helper()
	return NewMetricFilter(MetricFilterRequires{
		WMeta:       wmeta,
		FilterStore: &metricFilterStore{bundle: &metricFilterBundle{excluded: excluded}},
	})
}

func TestMetricFilterExcludesByContainerID(t *testing.T) {
	containerID := "container-1"
	wmeta := &metricFilterWMeta{
		container: &workloadmeta.Container{EntityID: workloadmeta.EntityID{ID: containerID}},
		pod: &workloadmeta.KubernetesPod{
			EntityMeta: workloadmeta.EntityMeta{Annotations: map[string]string{
				"ad.datadoghq.com/orchestrator.metrics_exclude": "true",
			}},
		},
	}
	filter := newMetricFilter(t, wmeta, true)
	require.True(t, filter.IsExcluded(&testMetric{tags: []string{"container_id:" + containerID}}))
}

func TestMetricFilterExcludesByPodAndContainerName(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-1"},
		EntityMeta: workloadmeta.EntityMeta{
			Name: "gensim-orchestrator",
			Annotations: map[string]string{
				"ad.datadoghq.com/orchestrator.exclude": "true",
			},
		},
		ContainerStatuses: []workloadmeta.KubernetesContainerStatus{{
			ContainerID: "container-1",
			Name:        "orchestrator",
		}},
	}
	wmeta := &metricFilterWMeta{pod: pod}
	filter := newMetricFilter(t, wmeta, true)
	require.True(t, filter.IsExcluded(&testMetric{
		tags: []string{"pod_name:gensim-orchestrator", "kube_container_name:orchestrator"},
	}))
}

func TestMetricFilterKeepsNonContainerAndUnknownMetrics(t *testing.T) {
	wmeta := &metricFilterWMeta{}
	filter := newMetricFilter(t, wmeta, true)
	require.False(t, filter.IsExcluded(&testMetric{tags: []string{"host:agent"}}))
	require.False(t, filter.IsExcluded(&testMetric{tags: []string{"pod_name:missing", "kube_container_name:missing"}}))
}

func TestMetricFilterHonorsOnlyMetricsScopedExclusion(t *testing.T) {
	pod := &workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-1"},
		EntityMeta: workloadmeta.EntityMeta{Annotations: map[string]string{
			"ad.datadoghq.com/orchestrator.logs_exclude": "true",
		}},
		ContainerStatuses: []workloadmeta.KubernetesContainerStatus{{
			ContainerID: "container-1",
			Name:        "orchestrator",
		}},
	}
	wmeta := &metricFilterWMeta{pod: pod}
	filter := newMetricFilter(t, wmeta, false)
	require.False(t, filter.IsExcluded(&testMetric{
		tags: []string{"pod_name:gensim-orchestrator", "kube_container_name:orchestrator"},
	}))
}

func TestMetricFilterNilWhenDependenciesMissing(t *testing.T) {
	require.Nil(t, NewMetricFilter(MetricFilterRequires{}))
	require.Nil(t, NewMetricFilter(MetricFilterRequires{WMeta: &metricFilterWMeta{}}))
	require.Nil(t, NewMetricFilter(MetricFilterRequires{FilterStore: &metricFilterStore{}}))
}
