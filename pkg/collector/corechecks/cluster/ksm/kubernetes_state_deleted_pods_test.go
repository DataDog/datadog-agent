// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"testing"
	"time"

	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	ksmstore "github.com/DataDog/datadog-agent/pkg/kubestatemetrics/store"
)

// newDeletedPodsCheck returns a check with a mock sender, ready to process the
// metrics drained for deleted pods.
func newDeletedPodsCheck(t *testing.T, config *KSMConfig) (*KSMCheck, *mocksender.MockSender) {
	fakeTagger := taggerfxmock.SetupFakeTagger(t)

	k := newKSMCheck(core.NewCheckBase(CheckName), config, fakeTagger, nil)
	mocked := mocksender.NewMockSender(t, k.ID())
	mocked.SetupAcceptAll()
	k.processLabelJoins()

	return k, mocked
}

// deletionTimestampFamily returns the family that marks a pod as terminating.
func deletionTimestampFamily(namespace, pod, uid string, timestamp float64) ksmstore.DDMetricsFam {
	return ksmstore.DDMetricsFam{
		Type: "*v1.Pod",
		Name: kubePodDeletionTimestampMetric,
		ListMetrics: []ksmstore.DDMetric{{
			Labels: map[string]string{"namespace": namespace, "pod": pod, "uid": uid},
			Val:    timestamp,
		}},
	}
}

// podInfoFamily returns the metadata family that label joins use to add the
// node and owner tags.
func podInfoFamily(namespace, pod, uid, node, ownerKind, ownerName string) ksmstore.DDMetricsFam {
	return ksmstore.DDMetricsFam{
		Type: "*v1.Pod",
		Name: "kube_pod_info",
		ListMetrics: []ksmstore.DDMetric{{
			Labels: map[string]string{
				"namespace":       namespace,
				"pod":             pod,
				"uid":             uid,
				"node":            node,
				"created_by_kind": ownerKind,
				"created_by_name": ownerName,
			},
			Val: 1,
		}},
	}
}

func TestProcessDeletedPodMetrics_EmitsTerminating(t *testing.T) {
	k, mocked := newDeletedPodsCheck(t, &KSMConfig{
		LabelsMapper: defaultLabelsMapper(),
		LabelJoins:   defaultLabelJoins(),
	})

	deletedPods := [][]ksmstore.DDMetricsFam{{
		deletionTimestampFamily("default", "deleted-pod", "uid-deleted", 1700000000),
		// Metadata family: enriches the emitted point, but is not emitted.
		podInfoFamily("default", "deleted-pod", "uid-deleted", "node-1", "ReplicaSet", "deleted-pod-rs"),
		// A mapped, non-metadata family: it must not be emitted either.
		{
			Type: "*v1.Pod",
			Name: "kube_pod_container_resource_requests",
			ListMetrics: []ksmstore.DDMetric{{
				Labels: map[string]string{"namespace": "default", "pod": "deleted-pod", "uid": "uid-deleted", "container": "c", "resource": "cpu"},
				Val:    0.5,
			}},
		},
	}}

	k.processDeletedPodMetrics(mocked, deletedPods, time.Now())

	// Exactly one Gauge: kubernetes_state.pod.terminating, enriched by the
	// label joins of the pod's own metadata.
	mocked.AssertNumberOfCalls(t, "Gauge", 1)
	mocked.AssertMetric(t, "Gauge", "kubernetes_state.pod.terminating", 1, "node-1",
		[]string{
			"kube_namespace:default",
			"pod_name:deleted-pod",
			"uid:uid-deleted",
			"node:node-1",
			"kube_replica_set:deleted-pod-rs",
		})
}

func TestProcessDeletedPodMetrics_NoDeletionTimestamp(t *testing.T) {
	k, mocked := newDeletedPodsCheck(t, &KSMConfig{LabelsMapper: defaultLabelsMapper()})

	// No kube_pod_deletion_timestamp — nothing should be emitted.
	deletedPods := [][]ksmstore.DDMetricsFam{{{
		Type: "*v1.Pod",
		Name: "kube_pod_status_phase",
		ListMetrics: []ksmstore.DDMetric{{
			Labels: map[string]string{"namespace": "default", "pod": "p", "uid": "u"},
			Val:    1,
		}},
	}}}

	k.processDeletedPodMetrics(mocked, deletedPods, time.Now())
	mocked.AssertNumberOfCalls(t, "Gauge", 0)
}

func TestProcessDeletedPodMetrics_ClusterAggregatesOnly(t *testing.T) {
	k, mocked := newDeletedPodsCheck(t, &KSMConfig{
		LabelsMapper:      defaultLabelsMapper(),
		PodCollectionMode: clusterAggregatesOnlyPodCollection,
	})

	deletedPods := [][]ksmstore.DDMetricsFam{{
		deletionTimestampFamily("default", "p", "u", 1700000000),
	}}

	k.processDeletedPodMetrics(mocked, deletedPods, time.Now())

	// In cluster_aggregates_only mode, per-pod metrics must be suppressed.
	mocked.AssertNumberOfCalls(t, "Gauge", 0)
}

func TestProcessDeletedPodMetrics_Empty(t *testing.T) {
	k, mocked := newDeletedPodsCheck(t, &KSMConfig{LabelsMapper: defaultLabelsMapper()})

	k.processDeletedPodMetrics(mocked, nil, time.Now())
	mocked.AssertNumberOfCalls(t, "Gauge", 0)
}

func TestProcessDeletedPodMetrics_PerPodIsolation(t *testing.T) {
	k, mocked := newDeletedPodsCheck(t, &KSMConfig{
		LabelsMapper: defaultLabelsMapper(),
		LabelJoins:   defaultLabelJoins(),
	})

	// Two StatefulSet pod generations with the same name and namespace but
	// different UIDs, both deleted within one scrape. Label joins are keyed by
	// namespace+pod name, so each pod needs its own joiner to keep its own node.
	deletedPods := [][]ksmstore.DDMetricsFam{
		{
			deletionTimestampFamily("default", "sts-0", "uid-gen1", 1700000000),
			podInfoFamily("default", "sts-0", "uid-gen1", "node-old", "StatefulSet", "sts"),
		},
		{
			deletionTimestampFamily("default", "sts-0", "uid-gen2", 1700001000),
			podInfoFamily("default", "sts-0", "uid-gen2", "node-new", "StatefulSet", "sts"),
		},
	}

	k.processDeletedPodMetrics(mocked, deletedPods, time.Now())

	// One point per deleted pod, each carrying its own node — a contaminated
	// joiner would put the other generation's node on the point.
	mocked.AssertNumberOfCalls(t, "Gauge", 2)
	mocked.AssertMetric(t, "Gauge", "kubernetes_state.pod.terminating", 1, "node-old",
		[]string{"uid:uid-gen1", "node:node-old", "pod_name:sts-0"})
	mocked.AssertMetric(t, "Gauge", "kubernetes_state.pod.terminating", 1, "node-new",
		[]string{"uid:uid-gen2", "node:node-new", "pod_name:sts-0"})
	mocked.AssertMetricNotTaggedWith(t, "Gauge", "kubernetes_state.pod.terminating",
		[]string{"node:node-old", "node:node-new"})
}
