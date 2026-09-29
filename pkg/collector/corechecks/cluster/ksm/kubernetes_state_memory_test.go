// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"context"
	"fmt"
	"runtime"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

// memoryBenchPod builds a pod shaped like a typical Deployment replica: a
// ReplicaSet owner, standard labels, a couple of annotations, one container with
// requests and limits, Running with the usual conditions.
func memoryBenchPod(i int) *corev1.Pod {
	deployment := fmt.Sprintf("svc-%04d", i/10)
	replicaSet := deployment + "-7d9f8b6c5d"
	name := fmt.Sprintf("%s-%05d", replicaSet, i)
	isController := true
	started := metav1.NewTime(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: fmt.Sprintf("team-%02d", i%50),
			UID:       types.UID(fmt.Sprintf("00000000-0000-4000-8000-%012d", i)),
			Labels: map[string]string{
				"tags.datadoghq.com/env":     "prod",
				"tags.datadoghq.com/service": deployment,
				"tags.datadoghq.com/version": "1.2.3",
				"app.kubernetes.io/name":     deployment,
				"app.kubernetes.io/instance": deployment + "-a",
				"app.kubernetes.io/part-of":  "shop",
				"pod-template-hash":          "7d9f8b6c5d",
			},
			Annotations: map[string]string{
				"ad.datadoghq.com/tags":             `{"team":"payments"}`,
				"kubectl.kubernetes.io/restartedAt": "2026-09-01T00:00:00Z",
			},
			OwnerReferences:   []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "ReplicaSet", Name: replicaSet, Controller: &isController}},
			CreationTimestamp: started,
		},
		Spec: corev1.PodSpec{
			NodeName:          fmt.Sprintf("node-%03d", i%100),
			PriorityClassName: "high",
			Containers: []corev1.Container{{
				Name:  "app",
				Image: "registry.example.com/" + deployment + ":1.2.3",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m"), corev1.ResourceMemory: resource.MustParse("128Mi")},
					Limits:   corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("500m"), corev1.ResourceMemory: resource.MustParse("512Mi")},
				},
			}},
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			QOSClass:  corev1.PodQOSBurstable,
			HostIP:    "10.0.0.1",
			PodIP:     fmt.Sprintf("10.1.%d.%d", i/256%256, i%256),
			StartTime: &started,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodInitialized, Status: corev1.ConditionTrue},
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
				{Type: corev1.ContainersReady, Status: corev1.ConditionTrue},
				{Type: corev1.PodScheduled, Status: corev1.ConditionTrue},
			},
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app", Ready: true, Image: "registry.example.com/" + deployment + ":1.2.3",
				ImageID: "registry.example.com/" + deployment + "@sha256:0123456789abcdef", ContainerID: fmt.Sprintf("containerd://%064d", i),
				State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: started}},
			}},
		},
	}
}

func heapInUse() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// BenchmarkKSMPodStoreMemory measures, for N pods and the real kube-state-metrics
// generators and store, the heap held by the KSM pod store today, and what
// precomputing every emitted series' tags at object-change time would add:
// naively (one tag slice per series), with the tags shared by all series of an
// object stored once, and with repeated tag strings interned on top.
//
// Run with: -test.run=^$ -test.bench=KSMPodStoreMemory -test.benchtime=1x
func BenchmarkKSMPodStoreMemory(b *testing.B) {
	for _, pods := range []int{10_000} {
		b.Run(fmt.Sprintf("pods=%d", pods), func(b *testing.B) {
			for b.Loop() {
				measureKSMPodStoreMemory(b, pods)
			}
		})
	}
}

func measureKSMPodStoreMemory(b *testing.B, pods int) {
	// The fake clientset never sends the bookmark that client-go's watch-list
	// mode waits for: use a plain list + watch.
	b.Setenv("KUBE_FEATURE_WatchListClient", "false")

	objects := make([]k8sruntime.Object, 0, pods)
	for i := 0; i < pods; i++ {
		objects = append(objects, memoryBenchPod(i))
	}
	client := fake.NewClientset(objects...)

	config := &KSMConfig{LabelsMapper: defaultLabelsMapper(), LabelJoins: defaultLabelJoins()}
	check := newKSMCheck(core.NewCheckBase(CheckName), config, taggerfxmock.SetupFakeTagger(b), nil)
	check.setupLabelsAndAnnotationsAsTagsFunc()

	collectors := []string{"pods"}
	allowDenyList, err := allowdenylist.New(options.MetricSet{}, buildDeniedMetricsSet(collectors))
	require.NoError(b, err)
	require.NoError(b, allowDenyList.Parse())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	before := heapInUse()

	builder := kubestatemetrics.New()
	builder.WithKubeClient(client)
	builder.WithNamespaces(options.DefaultNamespaces)
	builder.WithFamilyGeneratorFilter(allowDenyList)
	builder.WithContext(ctx)
	builder.WithResync(0)
	builder.WithGenerateStoresFunc(builder.GenerateStores)
	builder.WithAllowAnnotations(map[string][]string{"pods": {"*"}})
	require.NoError(b, builder.WithAllowLabels(map[string][]string{"pods": {"*"}}))
	require.NoError(b, builder.WithEnabledResources(collectors))
	stores := builder.BuildStores()

	var store *ksmstore.MetricsStore
	for _, group := range stores {
		for _, s := range group {
			if ms, ok := s.(*ksmstore.MetricsStore); ok {
				store = ms
			}
		}
	}
	require.NotNil(b, store)
	require.Eventually(b, func() bool {
		return len(store.Push(func(f ksmstore.DDMetricsFam) bool { return f.Name == "kube_pod_info" }, ksmstore.GetAllMetrics)["kube_pod_info"]) == pods
	}, 2*time.Minute, 100*time.Millisecond)

	storeHeap := heapInUse() - before

	// Emitted series and their tags, computed as a check run does today. The
	// heap is measured from here, so everything below that stays alive is what
	// precomputed tags would cost.
	base := heapInUse()
	joiner := newLabelJoiner(check.instance.labelJoins)
	joiner.insertFamilies(store.Push(check.familyFilter, check.metricFilter))
	all := store.Push(ksmstore.GetAllFamilies, ksmstore.GetAllMetrics)

	objectIDs := map[string]int32{}
	var naive [][]string // (b) naive: one tag slice per series
	var objectOf []int32
	for name, families := range all {
		_, mapped := check.metricNamesMapper[name]
		_, transformed := check.metricTransformers[name]
		if !mapped && !transformed {
			continue
		}
		override := labelsMapperOverride(name)
		for _, family := range families {
			for _, m := range family.ListMetrics {
				_, tags := check.hostnameAndTags(m.Labels, joiner, override)
				key := m.Labels["namespace"] + "/" + m.Labels["pod"]
				id, found := objectIDs[key]
				if !found {
					id = int32(len(objectIDs))
					objectIDs[key] = id
				}
				naive = append(naive, tags)
				objectOf = append(objectOf, id)
			}
		}
	}

	naive = slices.Clip(naive)
	objectOf = slices.Clip(objectOf)
	naiveHeap := heapInUse() - base

	// (c) shared: the tags common to every series of an object stored once,
	// plus each series' extra tags.
	common := map[int32][]string{}
	for i, tags := range naive {
		if c, found := common[objectOf[i]]; found {
			common[objectOf[i]] = slices.DeleteFunc(c, func(t string) bool { return !slices.Contains(tags, t) })
		} else {
			common[objectOf[i]] = slices.Clone(tags)
		}
	}
	extras := make([][]string, len(naive))
	for i, tags := range naive {
		c := common[objectOf[i]]
		for _, t := range tags {
			if !slices.Contains(c, t) {
				extras[i] = append(extras[i], t)
			}
		}
	}
	seriesCount := len(naive)
	sharedHeap := heapInUse() - base

	// (d) shared + interned: repeated tag strings stored once (the intern
	// table itself is included, as a real implementation would keep it).
	intern := map[string]string{}
	internAll := func(tags []string) {
		for i, t := range tags {
			if v, found := intern[t]; found {
				tags[i] = v
			} else {
				t = string([]byte(t)) // own copy, detached from the original backing string
				intern[t] = t
				tags[i] = t
			}
		}
	}
	for _, c := range common {
		internAll(c)
	}
	for _, e := range extras {
		internAll(e)
	}
	internedHeap := heapInUse() - base

	b.ReportMetric(float64(seriesCount)/float64(pods), "series/pod")
	b.ReportMetric(float64(storeHeap)/float64(pods), "store_B/pod")
	b.ReportMetric(float64(naiveHeap)/float64(pods), "naive_B/pod")
	b.ReportMetric(float64(sharedHeap)/float64(pods), "shared_B/pod")
	b.ReportMetric(float64(internedHeap)/float64(pods), "interned_B/pod")
	b.ReportMetric(float64(len(intern)), "distinct_tags")

	runtime.KeepAlive(store)
	runtime.KeepAlive(common)
	runtime.KeepAlive(extras)
	runtime.KeepAlive(objectOf)
	runtime.KeepAlive(client)
}
