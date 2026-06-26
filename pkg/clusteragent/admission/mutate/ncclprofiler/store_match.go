// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package ncclprofiler

import (
	corev1 "k8s.io/api/core/v1"

	instrumentationhandlers "github.com/DataDog/datadog-agent/pkg/clusteragent/instrumentation/handlers"
)

// enabledLabelValue reports whether the opt-in label is present and, if so, "true".
func enabledLabelValue(pod *corev1.Pod) (val bool, exists bool) {
	v, ok := pod.GetLabels()[EnabledLabel]
	if !ok {
		return false, false
	}
	return v == "true", true
}

// ddiConfig returns the enabled ncclProfiler configuration for a pod, if the pod
// matches a DatadogInstrumentation ncclProfiler target. It is a no-op (false) when
// DDI mode is off or no store is set.
func (w *Webhook) ddiConfig(pod *corev1.Pod) (instrumentationhandlers.NCCLProfilerConfig, bool) {
	if !w.ddi || w.store == nil {
		return instrumentationhandlers.NCCLProfilerConfig{}, false
	}
	for _, t := range workloadTargetsFromPod(pod) {
		if cfg, ok := w.store.Get(t); ok && cfg.Enabled {
			return cfg, true
		}
	}
	return instrumentationhandlers.NCCLProfilerConfig{}, false
}

// workloadTargetsFromPod resolves the candidate ncclProfiler targets a pod could
// match: each direct owner reference (RayCluster/PyTorchJob/Job/StatefulSet/DaemonSet)
// and the pod's namespace as the fallback. Owner refs are on the pod at admission
// time, so no API lookup is needed. Indirectly-owned workloads (Deployment via
// ReplicaSet, CronJob via Job) are intentionally not resolved — use a namespace target.
func workloadTargetsFromPod(pod *corev1.Pod) []instrumentationhandlers.NCCLProfilerTarget {
	ns := pod.Namespace
	var targets []instrumentationhandlers.NCCLProfilerTarget
	for _, ref := range pod.OwnerReferences {
		targets = append(targets, instrumentationhandlers.NCCLProfilerTarget{Kind: ref.Kind, Namespace: ns, Name: ref.Name})
	}
	// Namespace-scope fallback (CR targeting Kind "Namespace").
	targets = append(targets, instrumentationhandlers.NCCLProfilerTarget{Kind: instrumentationhandlers.NamespaceKind, Namespace: ns, Name: ns})
	return targets
}
