// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package cache

import (
	"container/list"
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/info"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

func TestIsRoot(t *testing.T) {
	for _, tt := range []struct {
		span *pb.Span
		want bool
	}{
		{&pb.Span{Name: "a", ParentID: 1}, false},
		{&pb.Span{Name: "b", ParentID: 0}, true},
		{&pb.Span{Name: "c", ParentID: 2, Metrics: map[string]float64{tagRootSpan: 3}}, true},
		{&pb.Span{Name: "d", ParentID: 0, Metrics: map[string]float64{tagRootSpan: 3}}, true},
	} {
		if isRoot(tt.span) != tt.want {
			t.Fatal(tt.span.Name)
		}
	}
}

func TestReassemblerAdd(t *testing.T) {
	ts := &info.TagStats{Tags: info.Tags{Lang: "go"}}
	out := make(chan *EvictedTrace, 1)
	r := newReassembler(out, 100000)
	defer r.Stop()

	span12 := testSpan(1, 1, 2)
	span13 := testSpan(1, 1, 3)

	// first trace
	addTime1 := time.Now()
	r.Add(&Item{
		Spans:  []*pb.Span{span12, span13},
		Source: ts,
	})
	shouldContain(t, r, &Item{
		Spans:   []*pb.Span{span12, span13},
		Source:  ts,
		lastmod: addTime1,
		key:     1,
	})

	span14 := testSpan(1, 2, 4)
	span15 := testSpan(1, 1, 5)

	// more items in first
	addTime1 = time.Now()
	r.Add(&Item{
		Spans: []*pb.Span{span14, span15},
	})
	shouldContain(t, r, &Item{
		Spans:   []*pb.Span{span12, span13, span14, span15},
		Source:  ts,
		lastmod: addTime1,
		key:     1,
	})

	// second trace
	ts2 := &info.TagStats{Tags: info.Tags{Lang: "python"}}
	span22 := testSpan(2, 2, 2)
	addTime2 := time.Now()
	r.Add(&Item{
		Spans:  []*pb.Span{span22},
		Source: ts2,
	})
	shouldContain(t, r, &Item{
		Spans:   []*pb.Span{span12, span13, span14, span15},
		Source:  ts,
		lastmod: addTime1,
		key:     1,
	}, &Item{
		Spans:   []*pb.Span{span22},
		lastmod: addTime2,
		Source:  ts2,
		key:     2,
	})

	// another item in first, order gets switched
	span16 := testSpan(1, 1, 6)
	addTime1 = time.Now()
	r.Add(&Item{
		Spans: []*pb.Span{span16},
	})
	shouldContain(t, r, &Item{
		Spans:   []*pb.Span{span22},
		lastmod: addTime2,
		Source:  ts2,
		key:     2,
	}, &Item{
		Spans:   []*pb.Span{span12, span13, span14, span15, span16},
		Source:  ts,
		lastmod: addTime1,
		key:     1,
	})

	// another item in second, order gets switched again
	span23 := testSpan(2, 2, 3)
	addTime2 = time.Now()
	r.Add(&Item{
		Spans: []*pb.Span{span23},
	})
	shouldContain(t, r, &Item{
		Spans:   []*pb.Span{span12, span13, span14, span15, span16},
		Source:  ts,
		lastmod: addTime1,
		key:     1,
	}, &Item{
		Spans:   []*pb.Span{span22, span23},
		lastmod: addTime2,
		Source:  ts2,
		key:     2,
	})
}

func TestReassemblerEvictReasonStopping(t *testing.T) {
	out := make(chan *EvictedTrace, 10)
	r := NewReassembler(out, 100000)

	// two traces
	addTime := time.Now()
	span1 := testSpan(1, 1, 1)
	span2 := testSpan(2, 1, 1)
	r.Add(&Item{Spans: []*pb.Span{span1, span2}})
	shouldContain(t, r, &Item{
		Spans:   []*pb.Span{span1},
		lastmod: addTime,
		key:     1,
	}, &Item{
		Spans:   []*pb.Span{span2},
		lastmod: addTime,
		key:     2,
	})

	r.Stop()

	shouldEvict(t, out, &EvictedTrace{
		Reason: EvictReasonStopping,
		Spans:  []*pb.Span{span2},
	})
	shouldEvict(t, out, &EvictedTrace{
		Reason: EvictReasonStopping,
		Spans:  []*pb.Span{span1},
	})
}

func TestReassemblerEvictReasonSpace(t *testing.T) {
	out := make(chan *EvictedTrace, 1)
	span1 := testSpan(1, 1, 1)
	span2 := testSpan(1, 1, 2)
	span3 := testSpan(3, 1, 3)
	r := newReassembler(out, span1.Msgsize()+span2.Msgsize())
	defer r.Stop()

	r.Add(&Item{Spans: []*pb.Span{span1}})
	if r.Len() != 1 {
		t.Fatalf("bad length: %d", r.Len())
	}
	addTime := time.Now()
	r.Add(&Item{Spans: []*pb.Span{span2}})
	if r.Len() != 1 {
		t.Fatalf("bad length: %d", r.Len())
	}

	// span3 pushes us overboard and we lost the trace above to make room
	r.Add(&Item{Spans: []*pb.Span{span3}})
	shouldEvict(t, out, &EvictedTrace{
		Reason: EvictReasonSpace,
		Spans:  []*pb.Span{span1, span2},
	})
	shouldContain(t, r, &Item{
		Spans:   []*pb.Span{span3},
		lastmod: addTime,
		key:     3,
	})
}

func TestReassemblerEvictReasonRoot(t *testing.T) {
	out := make(chan *EvictedTrace, 1)
	r := newReassembler(out, 100000)
	defer r.Stop()

	// two traces
	now := time.Now()
	span1 := testSpan(1, 1, 1)
	span2 := testSpan(2, 1, 1)
	r.addWithTime(now, &Item{Spans: []*pb.Span{span1, span2}})
	shouldContain(t, r, &Item{
		Spans:   []*pb.Span{span1},
		lastmod: now,
		key:     1,
	}, &Item{
		Spans:   []*pb.Span{span2},
		lastmod: now,
		key:     2,
	})

	// one root
	root1 := testSpan(1, 0, 5)
	r.addWithTime(now, &Item{Spans: []*pb.Span{root1}})
	shouldEvict(t, out, &EvictedTrace{
		Reason: EvictReasonRoot,
		Spans:  []*pb.Span{span1, root1},
	})
	shouldContain(t, r, &Item{
		Spans:   []*pb.Span{span2},
		lastmod: now,
		key:     2,
	})

	// new trace with root
	root3 := testSpan(3, 0, 1)
	span32 := testSpan(3, 1, 2)
	span33 := testSpan(3, 1, 3)
	r.addWithTime(now, &Item{Spans: []*pb.Span{root3, span32, span33}})
	shouldEvict(t, out, &EvictedTrace{
		Reason: EvictReasonRoot,
		Spans:  []*pb.Span{root3, span32, span33},
	})
	shouldContain(t, r, &Item{
		Spans:   []*pb.Span{span2},
		lastmod: now,
		key:     2,
	})
}

func TestReassemblerSweep(t *testing.T) {
	out := make(chan *EvictedTrace, 1)
	r := newReassembler(out, 100000)
	defer r.Stop()
	tick := make(chan time.Time)
	stopped := make(chan struct{})
	go func() {
		go r.sweepIdle(tick)
		close(stopped)
	}()

	now := time.Now()
	span1 := testSpan(1, 1, 1)
	span2 := testSpan(2, 1, 1)
	r.addWithTime(now, &Item{Spans: []*pb.Span{span1, span2}})
	tick <- time.Now()
	shouldContain(t, r, &Item{
		Spans:   []*pb.Span{span1},
		lastmod: now,
		key:     1,
	}, &Item{
		Spans:   []*pb.Span{span2},
		lastmod: now,
		key:     2,
	})

	expireTime := now.Add(-MaxTraceIdle - time.Second)
	span3 := testSpan(1, 1, 2)
	span4 := testSpan(2, 1, 2)
	span5 := testSpan(3, 1, 3)
	r.addWithTime(expireTime, &Item{Spans: []*pb.Span{span5}})
	r.addWithTime(expireTime, &Item{Spans: []*pb.Span{span4}})
	r.addWithTime(now, &Item{Spans: []*pb.Span{span3}})
	tick <- time.Now()
	shouldEvict(t, out, &EvictedTrace{
		Reason: EvictReasonIdle,
		Spans:  []*pb.Span{span5},
	})
	shouldEvict(t, out, &EvictedTrace{
		Reason: EvictReasonIdle,
		Spans:  []*pb.Span{span2, span4},
	})
	shouldContain(t, r, &Item{
		Spans:   []*pb.Span{span1, span3},
		lastmod: now,
		key:     1,
	})

	close(tick)
	<-stopped
}

func TestShouldContain(t *testing.T) {
	// TODO
}

func TestMetrics(t *testing.T) {
	// TODO
}

func testSpan(traceID, parentID, spanID uint64) *pb.Span {
	return &pb.Span{
		Name:     fmt.Sprintf("test-span-%d-%d-%d", traceID, parentID, spanID),
		TraceID:  traceID,
		SpanID:   spanID,
		ParentID: parentID,
	}
}

// shouldContain asserts that the given Reassembler contains the items specified. The items size must not be specified
// and will be automatically computed. The lastmod property is the minimum permitted time and must not be exact.
func shouldContain(t *testing.T, r *Reassembler, items ...*Item) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(items) != r.ll.Len() {
		t.Fatalf("wanted %d items, got %d", len(items), r.ll.Len())
	}
	var (
		totalSize int
		el        *list.Element
	)
	now := time.Now()
	for _, want := range items {
		if el == nil {
			el = r.ll.Back()
		} else {
			el = el.Prev()
		}
		if el == nil {
			t.Fatal("not found")
		}
		got := el.Value.(*Item)
		if !reflect.DeepEqual(want.Spans, got.Spans) {
			t.Fatalf("span contents differ for %d:\n%v !=\n%v", want.key, want.Spans, got.Spans)
		}
		if !reflect.DeepEqual(want.Source, got.Source) {
			t.Fatalf("item source differs for key %d: %v != %v", want.key, want.Source, got.Source)
		}
		if want.key != got.key {
			t.Fatalf("item key differs %d != %d", want.key, got.key)
		}
		if got.lastmod.After(now) || got.lastmod.Before(want.lastmod) {
			t.Fatalf("unexpected time for %d: %s", got.key, got.lastmod)
		}
		wantSize := spanSize(want.Spans...)
		if wantSize != got.size {
			t.Fatalf("item size differs %d != %d", want.size, got.size)
		}
		totalSize += wantSize
	}
	if r.size != totalSize {
		t.Fatal("size mismatch")
	}
}

func shouldEvict(t *testing.T, out <-chan *EvictedTrace, want *EvictedTrace) {
	select {
	case got := <-out:
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("%v != %v", got, want)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out")
	}
}

func spanSize(spans ...*pb.Span) int {
	var size int
	for _, span := range spans {
		size += span.Msgsize()
	}
	return size
}
