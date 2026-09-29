// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/kube-state-metrics/v2/pkg/allowdenylist"
	"k8s.io/kube-state-metrics/v2/pkg/options"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	kubestatemetrics "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/builder"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

// seriesTagsTestObjects is a small cluster covering the owner kinds, nodes with
// the labels the default node joins read, and the workloads.
func seriesTagsTestObjects() []k8sruntime.Object {
	var objects []k8sruntime.Object
	isController := true
	for n := 0; n < 3; n++ {
		objects = append(objects, &corev1.Node{
			ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("node-%03d", n), UID: types.UID(fmt.Sprintf("node-uid-%d", n)),
				Labels: map[string]string{"topology.kubernetes.io/region": "eu-west-1", "topology.kubernetes.io/zone": fmt.Sprintf("eu-west-1%c", 'a'+n), "team": "infra"}},
			Status: corev1.NodeStatus{NodeInfo: corev1.NodeSystemInfo{KernelVersion: "6.1.0", KubeletVersion: "v1.33.0", ContainerRuntimeVersion: "containerd://2.0", OSImage: "Debian"}},
		})
	}
	for i := 0; i < 30; i++ {
		pod := memoryBenchPod(i)
		pod.Spec.NodeName = fmt.Sprintf("node-%03d", i%3)
		switch i % 5 {
		case 1: // StatefulSet
			pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "StatefulSet", Name: "db", Controller: &isController}}
		case 2: // Argo Rollout, owned through a ReplicaSet
			pod.Labels["rollouts-pod-template-hash"] = "7d9f8b6c5d"
		case 3: // Job of a CronJob
			pod.OwnerReferences = []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "Job", Name: "backup-28123456", Controller: &isController}}
		case 4: // no owner
			pod.OwnerReferences = nil
		}
		objects = append(objects, pod)
	}
	for d := 0; d < 3; d++ {
		name := fmt.Sprintf("svc-%04d", d)
		labels := map[string]string{"tags.datadoghq.com/env": "prod", "app.kubernetes.io/name": name}
		objects = append(objects,
			&appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: fmt.Sprintf("team-%02d", d), UID: types.UID("deploy-" + name), Labels: labels}},
			&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: name + "-7d9f8b6c5d", Namespace: fmt.Sprintf("team-%02d", d), UID: types.UID("rs-" + name), Labels: labels,
				OwnerReferences: []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "Deployment", Name: name, Controller: &isController}}}},
		)
	}
	objects = append(objects, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "backup-28123456", Namespace: "team-03", UID: "job-uid",
		OwnerReferences: []metav1.OwnerReference{{APIVersion: "batch/v1", Kind: "CronJob", Name: "backup", Controller: &isController}}}})
	return objects
}

// TestPrecomputedSeriesTagsMatchPerRunTags builds real stores with the
// precomputation on, and checks that every emitted series gets exactly the
// hostname and tags the per-run path gives it.
func TestPrecomputedSeriesTagsMatchPerRunTags(t *testing.T) {
	for _, tc := range []struct {
		name              string
		labelsAsTags      map[string]map[string]string
		annotationsAsTags map[string]map[string]string
	}{
		{name: "default joins"},
		{
			name:              "labels and annotations as tags, pod (same object) and node (other object)",
			labelsAsTags:      map[string]map[string]string{"pod": {"app_kubernetes_io_part_of": "part_of"}, "node": {"team": "node_team"}},
			annotationsAsTags: map[string]map[string]string{"pod": {"kubectl_kubernetes_io_restartedAt": "restarted_at"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KUBE_FEATURE_WatchListClient", "false") // the fake clientset sends no watch-list bookmark

			config := &KSMConfig{LabelsMapper: defaultLabelsMapper(), LabelJoins: defaultLabelJoins(),
				LabelsAsTags: tc.labelsAsTags, AnnotationsAsTags: tc.annotationsAsTags, PrecomputeSeriesTags: true}
			fakeTagger := taggerfxmock.SetupFakeTagger(t)
			fakeTagger.SetTags(taggertypes.NewEntityID(taggertypes.KubernetesMetadata, string(util.GenerateKubeMetadataEntityID("", "namespaces", "", "team-01"))),
				"test", []string{"team:payments"}, nil, nil, nil)
			k := newKSMCheck(core.NewCheckBase(CheckName), config, fakeTagger, nil)
			k.setupLabelsAndAnnotationsAsTagsFunc()
			k.seriesTags = newSeriesTagsPlan(k.instance.labelJoins, aggregatorLabels(k.metricAggregators))

			collectors := []string{"pods", "nodes", "deployments", "replicasets", "jobs"}
			allowDenyList, err := allowdenylist.New(options.MetricSet{}, buildDeniedMetricsSet(collectors))
			require.NoError(t, err)
			require.NoError(t, allowDenyList.Parse())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			builder := kubestatemetrics.New()
			builder.WithKubeClient(fake.NewClientset(seriesTagsTestObjects()...))
			builder.WithNamespaces(options.DefaultNamespaces)
			builder.WithFamilyGeneratorFilter(allowDenyList)
			builder.WithContext(ctx)
			builder.WithResync(0)
			builder.WithGenerateStoresFunc(builder.GenerateStores)
			all := map[string][]string{}
			for _, c := range collectors {
				all[c] = []string{"*"}
			}
			builder.WithAllowAnnotations(all)
			require.NoError(t, builder.WithAllowLabels(all))
			require.NoError(t, builder.WithEnabledResources(collectors))
			builder.WithSeriesTagsFuncs(k.seriesTagsFuncs(k.seriesTags))
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
			require.Eventually(t, func() bool {
				return count("kube_pod_info") == 30 && count("kube_node_info") == 3 && count("kube_deployment_labels") == 3 &&
					count("kube_replicaset_labels") == 3 && count("kube_job_info") == 1
			}, 30*time.Second, 50*time.Millisecond)

			// One simulated run: per-run joins for the new path, every join for
			// the old one.
			joins, version := k.seriesTags.runJoins()
			k.run = newSeriesRun(version)
			runJoiner := k.buildJoiner(joins)
			fullJoiner := k.buildJoiner(k.instance.labelJoins)

			compared, precomputed := 0, 0
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
						for _, f := range families {
							for _, m := range f.ListMetrics {
								wantHost, wantTags := k.hostnameAndTags(m.Labels, fullJoiner, override)
								gotHost, gotTags := k.seriesHostnameAndTags(&f, m, runJoiner, override)
								slices.Sort(wantTags)
								slices.Sort(gotTags)
								assert.Equal(t, wantHost, gotHost, "hostname of %s %v", name, m.Labels)
								assert.Equal(t, slices.Compact(wantTags), slices.Compact(gotTags), "tags of %s %v", name, m.Labels)
								if _, _, ok := k.precomputedHostnameAndTags(&f, m, runJoiner, override); ok {
									precomputed++
								}
								compared++
							}
						}
					}
				}
			}
			require.NotZero(t, compared)
			t.Logf("%d series compared, %d from precomputed tags", compared, precomputed)
			assert.Greater(t, precomputed, compared*9/10, "most series must take the precomputed path")

			// Pod container series carry a node label, so the node joins are
			// cross-object for pods and resolved per run.
			cross, legacy := k.seriesTags.crossJoins("pod", version)
			assert.Contains(t, cross, "kube_node_labels")
			assert.False(t, legacy)
		})
	}
}

func TestKindOfStore(t *testing.T) {
	assert.Equal(t, "pod", kindOfStore("*v1.Pod"))
	assert.Equal(t, "persistentvolumeclaim", kindOfStore("*v1.PersistentVolumeClaim"))
	assert.Equal(t, "unstructured", kindOfStore("*unstructured.Unstructured"))
}

func TestSeriesTagsPlanClassification(t *testing.T) {
	k := newKSMCheck(core.NewCheckBase(CheckName), &KSMConfig{LabelsMapper: defaultLabelsMapper(), LabelJoins: defaultLabelJoins()}, taggerfxmock.SetupFakeTagger(t), nil)
	k.setupLabelsAndAnnotationsAsTagsFunc()
	plan := newSeriesTagsPlan(k.instance.labelJoins, aggregatorLabels(k.metricAggregators))

	podIntra := plan.intraJoins("pod")
	for _, join := range []string{"kube_pod_info", "kube_pod_labels", "kube_pod_status_phase", "kube_pod_status_reason"} {
		assert.Contains(t, podIntra, join)
	}
	assert.NotContains(t, podIntra, "kube_node_labels")
	assert.Contains(t, plan.intraJoins("node"), "kube_node_labels")
	assert.Contains(t, plan.intraJoins("job"), "kube_job_labels")

	// pod.count keeps the node label, so node joins stay available to aggregates;
	// no aggregate keeps the pod label, so pod joins are not resolved per run.
	assert.Contains(t, plan.forAggregate, "kube_node_labels")
	assert.NotContains(t, plan.forAggregate, "kube_pod_labels")
}

func TestTagsMultisetOps(t *testing.T) {
	// A tag present twice in a series must stay twice once split into common
	// and extra tags, as on the per-run path.
	a := []string{"x", "x", "y"}
	b := []string{"x", "z"}
	common := intersectTags(slices.Clone(a), b)
	assert.Equal(t, []string{"x"}, common)
	assert.Equal(t, []string{"x", "y"}, subtractTags(a, common))
	assert.Equal(t, []string{"z"}, subtractTags(b, common))

	common = intersectTags([]string{"x", "x"}, []string{"x", "x", "y"})
	assert.Equal(t, []string{"x", "x"}, common)
}

func TestSeriesTagsPlanVersion(t *testing.T) {
	k := newKSMCheck(core.NewCheckBase(CheckName), &KSMConfig{LabelsMapper: defaultLabelsMapper(), LabelJoins: defaultLabelJoins()}, taggerfxmock.SetupFakeTagger(t), nil)
	k.setupLabelsAndAnnotationsAsTagsFunc()
	plan := newSeriesTagsPlan(k.instance.labelJoins, aggregatorLabels(k.metricAggregators))

	_, version := plan.runJoins()
	_, legacy := plan.crossJoins("pod", version)
	assert.False(t, legacy)

	// A cross join learnt after the run took its joins sends the kind to the
	// per-run path for the rest of that run.
	plan.noteCrossJoins("pod", map[string]struct{}{"kube_node_labels": {}})
	_, legacy = plan.crossJoins("pod", version)
	assert.True(t, legacy)

	// Noting a known join again does not bump the version.
	_, version = plan.runJoins()
	plan.noteCrossJoins("pod", map[string]struct{}{"kube_node_labels": {}})
	_, legacy = plan.crossJoins("pod", version)
	assert.False(t, legacy)
}
