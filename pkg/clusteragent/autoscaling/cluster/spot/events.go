// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package spot

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// spotEventRecorder wraps record.EventRecorder with typed methods for spot scheduler events.
type spotEventRecorder struct {
	recorder record.EventRecorder
}

func newSpotEventRecorder(r record.EventRecorder) *spotEventRecorder {
	return &spotEventRecorder{recorder: r}
}

// rebalancingEviction emits a SpotRebalancingEviction event on the evicted pod.
func (e *spotEventRecorder) rebalancingEviction(namespace, name string, isSpot bool) {
	capacityType := "on-demand"
	if isSpot {
		capacityType = "spot"
	}
	e.recorder.Eventf(podRef(namespace, name), corev1.EventTypeWarning,
		EventReasonSpotRebalancingEviction,
		"Evicted %s pod for rebalancing", capacityType)
}

// fallbackEviction emits a SpotFallbackEviction event on the evicted pending spot pod.
func (e *spotEventRecorder) fallbackEviction(namespace, name string) {
	e.recorder.Event(podRef(namespace, name), corev1.EventTypeWarning,
		EventReasonSpotFallbackEviction,
		"Evicted timed-out pending spot pod for on-demand fallback")
}

// schedulingDisabled emits a SpotSchedulingDisabled event on the workload.
func (e *spotEventRecorder) schedulingDisabled(ref objectRef, disabledUntil time.Time) {
	obj, err := workloadRef(ref)
	if err != nil {
		log.Warnf("Cannot emit %s event: %v", EventReasonSpotSchedulingDisabled, err)
		return
	}
	e.recorder.Eventf(obj, corev1.EventTypeWarning,
		EventReasonSpotSchedulingDisabled,
		"Disabled spot scheduling until %s", disabledUntil.UTC().Format(time.RFC3339))
}

func podRef(namespace, name string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: namespace, Name: name}}
}

func workloadRef(ref objectRef) (runtime.Object, error) {
	for i := range spotWorkloadResources {
		if spotWorkloadResources[i].kind == ref.Kind {
			return spotWorkloadResources[i].newObject(ref.Namespace, ref.Name), nil
		}
	}
	return nil, fmt.Errorf("unknown workload kind %q", ref.Kind)
}
