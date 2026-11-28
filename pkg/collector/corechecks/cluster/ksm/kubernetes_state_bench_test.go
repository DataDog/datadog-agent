// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"fmt"
	"testing"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

// benchTagsSink keeps the compiler from optimizing the benchmarked tags away.
var benchTagsSink []string

// benchLabels returns the labels made of keys and values, failing b if they
// are invalid.
func benchLabels(b *testing.B, keys []string, values ...string) ksmstore.Labels {
	labels, err := ksmstore.NewLabels(keys, values)
	if err != nil {
		b.Fatal(err)
	}
	return labels
}

// BenchmarkHostnameAndTagsWithLabelJoins measures the tags of the container
// series of a run, with the default label joins, and the check tags the sender
// appends to them.
func BenchmarkHostnameAndTagsWithLabelJoins(b *testing.B) {
	fakeTagger := taggerfxmock.SetupFakeTagger(b)
	k := newKSMCheck(core.NewCheckBase(CheckName), &KSMConfig{
		LabelsMapper: defaultLabelsMapper(),
		LabelJoins:   defaultLabelJoins(),
		Tags:         []string{"team:containers"},
	}, fakeTagger, nil)
	k.processLabelJoins()
	k.clusterNameRFC1123 = "cluster"

	const nbPods = 2000
	phaseKeys := []string{"namespace", "pod", "uid", "phase"}
	infoKeys := []string{"namespace", "pod", "uid", "node", "created_by_kind", "created_by_name", "priority_class"}
	podLabelsKeys := []string{"namespace", "pod", "uid", "label_app_kubernetes_io_name", "label_app_kubernetes_io_instance", "label_tags_datadoghq_com_env", "label_tags_datadoghq_com_service", "label_tags_datadoghq_com_version"}
	containerKeys := []string{"namespace", "pod", "uid", "container", "reason"}

	for i := range 50 {
		namespace := fmt.Sprintf("namespace-%d", i)
		id := types.NewEntityID(types.KubernetesMetadata, string(util.GenerateKubeMetadataEntityID("", "namespaces", "", namespace)))
		fakeTagger.SetTags(id, "workloadmeta-kubernetes_metadata", []string{"team:" + namespace, "env:prod"}, nil, nil, nil)
	}

	var infos, podLabels, phases []ksmstore.DDMetric
	var series []ksmstore.Labels
	for i := range nbPods {
		namespace := fmt.Sprintf("namespace-%d", i%50)
		pod := fmt.Sprintf("app-6768ddc4d-%05d", i)
		uid := fmt.Sprintf("bec19172-8abf-11ea-8546-%012d", i)
		infos = append(infos, ksmstore.DDMetric{Val: 1, Labels: benchLabels(b, infoKeys,
			namespace, pod, uid, fmt.Sprintf("node-%d", i%100), "ReplicaSet", "app-6768ddc4d", "default")})
		podLabels = append(podLabels, ksmstore.DDMetric{Val: 1, Labels: benchLabels(b, podLabelsKeys,
			namespace, pod, uid, "app", "app-1", "prod", "app", "1.2.3")})
		phases = append(phases, ksmstore.DDMetric{Val: 1, Labels: benchLabels(b, phaseKeys,
			namespace, pod, uid, "Running")})
		for _, container := range []string{"app", "sidecar"} {
			for _, reason := range []string{"ContainerCreating", "CrashLoopBackOff", "ErrImagePull", "ImagePullBackOff", "CreateContainerConfigError"} {
				series = append(series, benchLabels(b, containerKeys, namespace, pod, uid, container, reason))
			}
		}
	}

	labelJoiner := newLabelJoiner(k.instance.labelJoins)
	labelJoiner.insertFamilies(map[string][]ksmstore.DDMetricsFam{
		"kube_pod_info":         {{Name: "kube_pod_info", ListMetrics: infos}},
		"kube_pod_labels":       {{Name: "kube_pod_labels", ListMetrics: podLabels}},
		"kube_pod_status_phase": {{Name: "kube_pod_status_phase", ListMetrics: phases}},
	})

	lMapperOverride := labelsMapperOverride("kube_pod_container_status_waiting_reason")
	b.ReportAllocs()
	for b.Loop() {
		for _, labels := range series {
			_, tags := k.hostnameAndTags("kube_pod_container_status_waiting_reason", labels, labelJoiner, lMapperOverride)
			// The check tags the sender appends to the tags of every sample
			benchTagsSink = append(tags, k.instance.Tags...)
		}
		k.endStringCachesRun()
		// As Run does on every run
		k.resetRunCaches()
	}
	b.ReportMetric(float64(len(series)), "series")
}
