// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes"
)

// countingTagger counts the tagger lookups made by the check.
type countingTagger struct {
	taggermock.Mock
	calls map[types.EntityID]int
}

func (c *countingTagger) Tag(entityID types.EntityID, cardinality types.TagCardinality) ([]string, error) {
	c.calls[entityID]++
	return c.Mock.Tag(entityID, cardinality)
}

func setWorkloadTags(t *testing.T, fakeTagger taggermock.Mock, target kubernetes.WorkloadTarget, low ...string) {
	t.Helper()
	entityID, ok := workloadTaggerEntityID(target)
	if !ok {
		t.Fatalf("no tagger entity for %v", target)
	}
	fakeTagger.SetTags(entityID, "workloadmeta-test", low, nil, nil, nil)
}

func TestSeriesWorkloadTarget(t *testing.T) {
	target := func(kind, name string) kubernetes.WorkloadTarget {
		return kubernetes.WorkloadTarget{Kind: kind, Namespace: "ns", Name: name}
	}
	tests := []struct {
		name          string
		labels        map[string]string
		namespace     string
		ownerKind     string
		ownerName     string
		isArgoRollout bool
		want          kubernetes.WorkloadTarget
		wantFound     bool
	}{
		{name: "pod of a deployment", namespace: "ns", ownerKind: "ReplicaSet", ownerName: "web-7d9f8b6c5d", want: target("Deployment", "web"), wantFound: true},
		{name: "pod of a rollout", namespace: "ns", ownerKind: "ReplicaSet", ownerName: "web-7d9f8b6c5d", isArgoRollout: true, want: target("Rollout", "web"), wantFound: true},
		{name: "pod of a statefulset", namespace: "ns", ownerKind: "StatefulSet", ownerName: "db", want: target("StatefulSet", "db"), wantFound: true},
		{name: "deployment series", namespace: "ns", labels: map[string]string{deploymentKey: "web"}, want: target("Deployment", "web"), wantFound: true},
		{name: "statefulset series", namespace: "ns", labels: map[string]string{statefulSetKey: "db"}, want: target("StatefulSet", "db"), wantFound: true},
		{name: "pod of a daemonset", namespace: "ns", ownerKind: "DaemonSet", ownerName: "agent"},
		{name: "pod of a job", namespace: "ns", ownerKind: "Job", ownerName: "backup-28123456"},
		{name: "replicaset name without a hash", namespace: "ns", ownerKind: "ReplicaSet", ownerName: "standalone"},
		{name: "no namespace", labels: map[string]string{deploymentKey: "web"}},
		{name: "unrelated series", namespace: "ns", labels: map[string]string{"node": "node-a"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, found := seriesWorkloadTarget(tt.labels, tt.namespace, tt.ownerKind, tt.ownerName, tt.isArgoRollout)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestKSMCheck_hostnameAndTagsAutoscalerKinds(t *testing.T) {
	const namespace = "ns"
	web := kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: namespace, Name: "web"}
	db := kubernetes.WorkloadTarget{Kind: kubernetes.StatefulSetKind, Namespace: namespace, Name: "db"}
	checkout := kubernetes.WorkloadTarget{Kind: kubernetes.RolloutKind, Namespace: namespace, Name: "checkout"}

	type pod struct {
		name, ownerKind, ownerName string
		argo                       bool
	}
	pods := []pod{
		{name: "web-7d9f8b6c5d-x2q4z", ownerKind: "ReplicaSet", ownerName: "web-7d9f8b6c5d"},
		{name: "db-0", ownerKind: "StatefulSet", ownerName: "db"},
		{name: "checkout-5b6c7d8f9-abcde", ownerKind: "ReplicaSet", ownerName: "checkout-5b6c7d8f9", argo: true},
		{name: "agent-x7k2p", ownerKind: "DaemonSet", ownerName: "agent"},
	}

	tests := []struct {
		name   string
		labels map[string]string
		want   []string
	}{
		{name: "pod of a deployment", labels: map[string]string{"pod": pods[0].name}, want: []string{"kube_autoscaler_kind:hpa", "kube_autoscaler_kind:vpa"}},
		{name: "container of a deployment's pod", labels: map[string]string{"pod": pods[0].name, "container": "app"}, want: []string{"kube_autoscaler_kind:hpa", "kube_autoscaler_kind:vpa"}},
		{name: "pod of a statefulset", labels: map[string]string{"pod": pods[1].name}, want: []string{"kube_autoscaler_kind:keda"}},
		{name: "pod of a rollout", labels: map[string]string{"pod": pods[2].name}, want: []string{"kube_autoscaler_kind:dpa"}},
		{name: "pod of a daemonset", labels: map[string]string{"pod": pods[3].name}},
		{name: "deployment series", labels: map[string]string{deploymentKey: "web"}, want: []string{"kube_autoscaler_kind:hpa", "kube_autoscaler_kind:vpa"}},
		{name: "statefulset series", labels: map[string]string{statefulSetKey: "db"}, want: []string{"kube_autoscaler_kind:keda"}},
		{name: "deployment without autoscaler", labels: map[string]string{deploymentKey: "batch"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &KSMConfig{LabelsMapper: defaultLabelsMapper(), LabelJoins: defaultLabelJoins()}
			fakeTagger := taggerfxmock.SetupFakeTagger(t)
			// The workload entities may carry other tags (labels as tags):
			// only kube_autoscaler_kind must reach KSM series.
			setWorkloadTags(t, fakeTagger, web, "kube_autoscaler_kind:hpa", "kube_autoscaler_kind:vpa", "team:payments")
			setWorkloadTags(t, fakeTagger, db, "kube_autoscaler_kind:keda")
			setWorkloadTags(t, fakeTagger, checkout, "kube_autoscaler_kind:dpa")
			setWorkloadTags(t, fakeTagger, kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: namespace, Name: "batch"}, "team:data")

			check := newKSMCheck(core.NewCheckBase(CheckName), config, fakeTagger, nil)
			check.processLabelJoins()
			labelJoiner := newLabelJoiner(config.labelJoins)
			for _, p := range pods {
				podLabels := map[string]string{namespaceKey: namespace, "pod": p.name}
				if p.argo {
					podLabels[argoRolloutLabelName] = "5b6c7d8f9"
				}
				labelJoiner.insertFamily(ksmstore.DDMetricsFam{Name: "kube_pod_labels", ListMetrics: []ksmstore.DDMetric{{Labels: podLabels}}})
				labelJoiner.insertFamily(ksmstore.DDMetricsFam{Name: "kube_pod_info", ListMetrics: []ksmstore.DDMetric{{Labels: map[string]string{
					namespaceKey: namespace, "pod": p.name, createdByKindKey: p.ownerKind, createdByNameKey: p.ownerName,
				}}}})
			}

			labels := map[string]string{namespaceKey: namespace}
			for key, value := range tt.labels {
				labels[key] = value
			}
			_, tags := check.hostnameAndTags(labels, labelJoiner, nil)

			var got []string
			for _, tag := range tags {
				if strings.HasPrefix(tag, autoscalerKindTagPrefix) {
					got = append(got, tag)
				}
			}
			assert.ElementsMatch(t, tt.want, got)
			assert.NotContains(t, tags, "team:payments")
			assert.NotContains(t, tags, "team:data")
		})
	}
}

func TestKSMCheck_autoscalerTagsCachedPerRun(t *testing.T) {
	web := kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: "ns", Name: "web"}
	fakeTagger := taggerfxmock.SetupFakeTagger(t)
	setWorkloadTags(t, fakeTagger, web, "kube_autoscaler_kind:hpa")
	counting := &countingTagger{Mock: fakeTagger, calls: map[types.EntityID]int{}}
	check := newKSMCheck(core.NewCheckBase(CheckName), &KSMConfig{}, counting, nil)

	entityID, _ := workloadTaggerEntityID(web)
	for range 3 {
		assert.Equal(t, []string{"kube_autoscaler_kind:hpa"}, check.autoscalerTags(web))
	}
	assert.Equal(t, 1, counting.calls[entityID], "one tagger lookup per workload per run")

	// Run starts with an empty cache, so a change is picked up by the next run.
	setWorkloadTags(t, fakeTagger, web, "kube_autoscaler_kind:hpa", "kube_autoscaler_kind:vpa")
	check.autoscalerTagsCache = nil
	assert.ElementsMatch(t, []string{"kube_autoscaler_kind:hpa", "kube_autoscaler_kind:vpa"}, check.autoscalerTags(web))
	assert.Equal(t, 2, counting.calls[entityID])
}

func TestWorkloadTaggerEntityID(t *testing.T) {
	id, ok := workloadTaggerEntityID(kubernetes.WorkloadTarget{Kind: kubernetes.DeploymentKind, Namespace: "ns", Name: "web"})
	assert.True(t, ok)
	assert.Equal(t, types.NewEntityID(types.KubernetesDeployment, "ns/web"), id)

	id, ok = workloadTaggerEntityID(kubernetes.WorkloadTarget{Kind: kubernetes.StatefulSetKind, Namespace: "ns", Name: "db"})
	assert.True(t, ok)
	assert.Equal(t, types.NewEntityID(types.KubernetesMetadata, string(util.GenerateKubeMetadataEntityID("apps", "statefulsets", "ns", "db"))), id)

	_, ok = workloadTaggerEntityID(kubernetes.WorkloadTarget{Kind: "DaemonSet", Namespace: "ns", Name: "agent"})
	assert.False(t, ok)
}
