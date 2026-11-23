// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package collectors

// QueuedTerminated returns the number of queued deleted resources, so that tests can wait for informer events.
func (q *TerminatedQueue) QueuedTerminated() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.resources)
}
