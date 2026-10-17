// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"slices"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/sbom"
)

func report(id, at string) *sbom.ScanResult {
	return &sbom.ScanResult{RequestID: id, GenerationMethod: at}
}

// queued returns the request IDs and generation methods of the reports q
// holds, which the tests use to tell reports apart.
func queued(q *sbomQueue) []string {
	var got []string
	for _, r := range q.take() {
		got = append(got, r.RequestID+"@"+r.GenerationMethod)
	}
	return got
}

func TestSBOMQueueKeepsLatestReport(t *testing.T) {
	q := newSBOMQueue(10)
	q.push(report("a", "1"))
	q.push(report("", "1"))
	q.push(report("a", "2"))

	if got, want := queued(q), []string{"a@2", "@1"}; !slices.Equal(got, want) {
		t.Errorf("queued = %v, want %v", got, want)
	}
	if got := queued(q); len(got) != 0 {
		t.Errorf("queued after take = %v, want none", got)
	}
}

func TestSBOMQueueDropsOldest(t *testing.T) {
	q := newSBOMQueue(2)
	if q.push(report("a", "1")) || q.push(report("b", "1")) {
		t.Errorf("a report was dropped below the size of the queue")
	}
	if !q.push(report("c", "1")) {
		t.Errorf("no report was dropped above the size of the queue")
	}

	if got, want := queued(q), []string{"b@1", "c@1"}; !slices.Equal(got, want) {
		t.Errorf("queued = %v, want %v", got, want)
	}
}

func TestSBOMQueueRequeueKeepsNewer(t *testing.T) {
	q := newSBOMQueue(10)
	q.push(report("a", "1"))
	q.push(report("b", "1"))
	sent := q.take()
	q.push(report("a", "2"))
	q.requeue(sent)

	if got, want := queued(q), []string{"a@2", "b@1"}; !slices.Equal(got, want) {
		t.Errorf("queued = %v, want %v", got, want)
	}
}

func TestSBOMQueueSignals(t *testing.T) {
	q := newSBOMQueue(10)
	q.push(report("a", "1"))
	q.push(report("b", "1"))

	select {
	case <-q.ready:
	default:
		t.Fatalf("no signal for a queued report")
	}
	select {
	case <-q.ready:
		t.Errorf("one signal per push, want one for all the reports waiting")
	default:
	}
}
