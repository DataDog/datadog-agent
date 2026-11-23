// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package collectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

func TestTerminatedQueue(t *testing.T) {
	deleted := &appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "rs"}}
	var queue TerminatedQueue

	// Without a gate, or with a disabled one, nothing is queued.
	queue.QueueTerminated(deleted)
	gate := &TerminatedResourceGate{}
	queue.SetTerminatedResourceGate(gate)
	queue.QueueTerminated(deleted)
	assert.Empty(t, queue.DrainTerminated())

	// An enabled gate queues deleted resources, and the last known state of a missed deletion.
	gate.Enable()
	queue.QueueTerminated(deleted)
	queue.QueueTerminated(cache.DeletedFinalStateUnknown{Obj: deleted})
	queue.QueueTerminated(cache.DeletedFinalStateUnknown{Obj: nil})
	queue.QueueTerminated((*appsv1.ReplicaSet)(nil))
	assert.Equal(t, []interface{}{deleted, deleted}, queue.DrainTerminated())
	assert.Empty(t, queue.DrainTerminated(), "draining empties the queue")

	gate.Disable()
	queue.QueueTerminated(deleted)
	assert.Empty(t, queue.DrainTerminated())
}
