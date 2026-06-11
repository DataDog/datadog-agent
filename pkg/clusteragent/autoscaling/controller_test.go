// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoscaling

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"k8s.io/client-go/util/workqueue"
)

// testProcessor records every Process call and optionally blocks via onProcess.
type testProcessor struct {
	mu        sync.Mutex
	calls     map[string]int
	onProcess func(key string)
}

func (p *testProcessor) Process(_ context.Context, key, _, _ string) ProcessResult {
	if p.onProcess != nil {
		p.onProcess(key)
	}
	p.mu.Lock()
	p.calls[key]++
	p.mu.Unlock()
	return NoRequeue
}

func (p *testProcessor) snapshot() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make(map[string]int, len(p.calls))
	for k, v := range p.calls {
		out[k] = v
	}
	return out
}

// newTestController builds a Controller wired with a real workqueue and a
// controllable synced gate. Items are enqueued and tracked before synced is
// released, mirroring what the informer does during the initial list.
// Run() is started in a background goroutine; t.Cleanup tears everything down.
func newTestController(t *testing.T, keys []string, numWorkers int, proc *testProcessor) *Controller {
	t.Helper()

	wq := workqueue.NewTypedRateLimitingQueueWithConfig(
		workqueue.DefaultTypedItemBasedRateLimiter[string](),
		workqueue.TypedRateLimitingQueueConfig[string]{Name: "test"},
	)

	synced := make(chan struct{})
	c := &Controller{
		processor: proc,
		ID:        "test",
		Workqueue: wq,
		IsLeader:  func() bool { return true },
		synced: func() bool {
			select {
			case <-synced:
				return true
			default:
				return false
			}
		},
	}

	for _, key := range keys {
		c.initTracker.add(key)
		wq.Add(key)
	}

	close(synced)

	ctx, cancel := context.WithCancel(context.Background())
	go c.Run(ctx, numWorkers)

	t.Cleanup(cancel)

	return c
}

// assertInitialSyncInvariant waits for InitialSyncDone and then verifies that
// every unique key from the initial list has been processed at least once.
func assertInitialSyncInvariant(t *testing.T, c *Controller, keys []string, proc *testProcessor) {
	t.Helper()

	require.Eventually(t, c.InitialSyncDone, 5*time.Second, time.Millisecond,
		"InitialSyncDone should become true")

	unique := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		unique[k] = struct{}{}
	}
	snap := proc.snapshot()
	for key := range unique {
		assert.GreaterOrEqual(t, snap[key], 1, "key %q should be processed at least once", key)
	}
}

func TestControllerInitialSync(t *testing.T) {
	tests := []struct {
		name    string
		keys    []string
		workers int
	}{
		{"no items", nil, 1},
		{"single item", []string{"ns/a"}, 1},
		{"multiple items single worker", []string{"ns/a", "ns/b", "ns/c", "ns/d", "ns/e"}, 1},
		{"multiple items multiple workers", []string{"ns/a", "ns/b", "ns/c", "ns/d", "ns/e"}, 4},
		{"duplicate keys", []string{"ns/a", "ns/a", "ns/b", "ns/b", "ns/c"}, 2},
		{"many items many workers", makeKeys(100), 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proc := &testProcessor{calls: make(map[string]int)}
			c := newTestController(t, tt.keys, tt.workers, proc)
			assertInitialSyncInvariant(t, c, tt.keys, proc)
		})
	}
}

func TestControllerInitialSync_WaitsForSlowItem(t *testing.T) {
	gate := make(chan struct{})
	proc := &testProcessor{
		calls: make(map[string]int),
		onProcess: func(key string) {
			if key == "ns/slow" {
				<-gate
			}
		},
	}

	keys := []string{"ns/fast-1", "ns/fast-2", "ns/slow"}
	c := newTestController(t, keys, 3, proc)

	require.Eventually(t, func() bool {
		snap := proc.snapshot()
		return snap["ns/fast-1"] > 0 && snap["ns/fast-2"] > 0
	}, 5*time.Second, time.Millisecond)

	assert.False(t, c.InitialSyncDone(),
		"InitialSyncDone should be false while an initial item is still pending")

	close(gate)

	assertInitialSyncInvariant(t, c, keys, proc)
}

func FuzzControllerInitialSync(f *testing.F) {
	f.Add(uint8(0), uint8(1), uint8(1))
	f.Add(uint8(1), uint8(1), uint8(1))
	f.Add(uint8(5), uint8(2), uint8(5))
	f.Add(uint8(20), uint8(4), uint8(5))
	f.Add(uint8(200), uint8(8), uint8(200))

	f.Fuzz(func(t *testing.T, numItems, numWorkers, numUnique uint8) {
		n := int(numItems)
		w := max(int(numWorkers%8), 1)
		u := max(int(numUnique), 1)

		keys := make([]string, n)
		for i := range keys {
			keys[i] = fmt.Sprintf("ns/obj-%d", i%u)
		}

		proc := &testProcessor{calls: make(map[string]int)}
		c := newTestController(t, keys, w, proc)
		assertInitialSyncInvariant(t, c, keys, proc)
	})
}

func makeKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("ns/obj-%d", i)
	}
	return keys
}
