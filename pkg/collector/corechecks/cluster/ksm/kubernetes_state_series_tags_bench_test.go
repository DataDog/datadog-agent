// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kube-state-metrics/v2/pkg/allowdenylist"
	"k8s.io/kube-state-metrics/v2/pkg/options"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	kubestatemetrics "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/builder"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

// BenchmarkSeriesTagsRun measures what a check run spends on the tags of the
// emitted series of pods and nodes: building the run's label joiner, then the
// hostname and tags of every series, with and without precompute_series_tags.
//
// Run with: -test.run=^$ -test.bench=SeriesTagsRun -test.benchmem
func BenchmarkSeriesTagsRun(b *testing.B) {
	for _, precompute := range []bool{false, true} {
		b.Run(fmt.Sprintf("precompute=%t/pods=2000", precompute), func(b *testing.B) {
			benchmarkSeriesTagsRun(b, precompute, 2000, 20)
		})
	}
}

func benchmarkSeriesTagsRun(b *testing.B, precompute bool, pods, nodes int) {
	b.Setenv("KUBE_FEATURE_WatchListClient", "false") // the fake clientset sends no watch-list bookmark

	objects := make([]k8sruntime.Object, 0, pods+nodes)
	for n := 0; n < nodes; n++ {
		objects = append(objects, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("node-%03d", n), UID: types.UID(fmt.Sprintf("node-uid-%d", n)),
				Labels: map[string]string{"topology.kubernetes.io/region": "eu-west-1", "topology.kubernetes.io/zone": "eu-west-1a"}},
		})
	}
	for i := 0; i < pods; i++ {
		pod := memoryBenchPod(i)
		pod.Spec.NodeName = fmt.Sprintf("node-%03d", i%nodes)
		objects = append(objects, pod)
	}

	config := &KSMConfig{LabelsMapper: defaultLabelsMapper(), LabelJoins: defaultLabelJoins(), PrecomputeSeriesTags: precompute}
	k := newKSMCheck(core.NewCheckBase(CheckName), config, taggerfxmock.SetupFakeTagger(b), nil)
	k.setupLabelsAndAnnotationsAsTagsFunc()

	collectors := []string{"pods", "nodes"}
	allowDenyList, err := allowdenylist.New(options.MetricSet{}, buildDeniedMetricsSet(collectors))
	require.NoError(b, err)
	require.NoError(b, allowDenyList.Parse())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	builder := kubestatemetrics.New()
	builder.WithKubeClient(fake.NewClientset(objects...))
	builder.WithNamespaces(options.DefaultNamespaces)
	builder.WithFamilyGeneratorFilter(allowDenyList)
	builder.WithContext(ctx)
	builder.WithResync(0)
	builder.WithGenerateStoresFunc(builder.GenerateStores)
	allowed := map[string][]string{}
	for _, c := range collectors {
		allowed[c] = []string{"*"}
	}
	builder.WithAllowAnnotations(allowed)
	require.NoError(b, builder.WithAllowLabels(allowed))
	require.NoError(b, builder.WithEnabledResources(collectors))
	if precompute {
		k.seriesTags = newSeriesTagsPlan(k.instance.labelJoins, aggregatorLabels(k.metricAggregators))
		builder.WithSeriesTagsFuncs(k.seriesTagsFuncs(k.seriesTags))
	}
	k.allStores = builder.BuildStores()

	count := func(family string) int {
		n := 0
		for _, stores := range k.allStores {
			for _, s := range stores {
				if ms, ok := s.(*ksmstore.MetricsStore); ok {
					n += len(ms.Push(func(f ksmstore.DDMetricsFam) bool { return f.Name == family }, ksmstore.GetAllMetrics)[family])
				}
			}
		}
		return n
	}
	require.Eventually(b, func() bool {
		return count("kube_pod_info") == pods && count("kube_node_info") == nodes
	}, 2*time.Minute, 100*time.Millisecond)

	// The series a run would send, read once: only the tag work is measured.
	type series struct {
		family   *ksmstore.DDMetricsFam
		metric   ksmstore.DDMetric
		override map[string]string
	}
	var emitted []series
	for _, stores := range k.allStores {
		for _, s := range stores {
			ms, ok := s.(*ksmstore.MetricsStore)
			if !ok {
				continue
			}
			for name, families := range ms.Push(ksmstore.GetAllFamilies, ksmstore.GetAllMetrics) {
				if !k.emitted(name) {
					continue
				}
				override := labelsMapperOverride(name)
				for fi := range families {
					for _, m := range families[fi].ListMetrics {
						emitted = append(emitted, series{&families[fi], m, override})
					}
				}
			}
		}
	}
	require.NotEmpty(b, emitted)
	b.ReportMetric(float64(len(emitted)), "series")

	b.ReportAllocs()
	for b.Loop() {
		var joiner *labelJoiner
		if precompute {
			joins, version := k.seriesTags.runJoins()
			k.run = newSeriesRun(version)
			joiner = k.buildJoiner(joins)
		} else {
			joiner = k.buildJoiner(k.instance.labelJoins)
		}
		for _, s := range emitted {
			k.seriesHostnameAndTags(s.family, s.metric, joiner, s.override)
		}
		k.run = nil
	}
}
