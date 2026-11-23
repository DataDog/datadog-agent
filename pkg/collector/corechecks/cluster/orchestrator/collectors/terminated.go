// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package collectors

import (
	"fmt"
	"reflect"
	"sync"

	"go.uber.org/atomic"
	"k8s.io/client-go/tools/cache"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TerminatedResourceGate reports whether collectors queue the resources that are reported as deleted. The check only
// queues them while it is the leader.
type TerminatedResourceGate struct {
	enabled atomic.Bool
}

// Enable starts queueing deleted resources.
func (g *TerminatedResourceGate) Enable() {
	g.enabled.Store(true)
}

// Disable stops queueing deleted resources.
func (g *TerminatedResourceGate) Disable() {
	g.enabled.Store(false)
}

// Enabled reports whether deleted resources are queued. A nil gate queues nothing.
func (g *TerminatedResourceGate) Enabled() bool {
	return g != nil && g.enabled.Load()
}

// TerminatedQueue queues the deleted resources of a collector until the collector bundle sends them. Collectors embed
// it.
type TerminatedQueue struct {
	mu        sync.Mutex
	gate      *TerminatedResourceGate
	resources []interface{}
}

// SetTerminatedResourceGate sets the gate that the queue checks before it queues a deleted resource.
func (q *TerminatedQueue) SetTerminatedResourceGate(gate *TerminatedResourceGate) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.gate = gate
}

// QueueTerminated queues a deleted resource, or the last known state of a resource whose deletion was missed. The gate
// is checked under the queue lock, so that a resource is either drained after the gate is disabled or not queued.
func (q *TerminatedQueue) QueueTerminated(obj interface{}) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if !q.gate.Enabled() {
		return
	}

	resource, err := getResource(obj)
	if err != nil {
		log.Warn(err)
		return
	}
	q.resources = append(q.resources, resource)
}

// DrainTerminated returns the queued resources and empties the queue.
func (q *TerminatedQueue) DrainTerminated() []interface{} {
	q.mu.Lock()
	defer q.mu.Unlock()
	resources := q.resources
	q.resources = nil
	return resources
}

// getResource checks if the resource is of type DeletedFinalStateUnknown
// and returns the underlying object if it is, or an error if the object is nil.
func getResource(obj interface{}) (interface{}, error) {
	resource := obj
	if deletedState, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		resource = deletedState.Obj
	}

	if resource == nil || (reflect.ValueOf(resource).Kind() == reflect.Pointer && reflect.ValueOf(resource).IsNil()) {
		return nil, fmt.Errorf("object is nil, skipping, got type: %T", obj)
	}

	return resource, nil
}
