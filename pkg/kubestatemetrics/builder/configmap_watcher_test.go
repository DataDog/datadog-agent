// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package builder

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiwatch "k8s.io/apimachinery/pkg/watch"
)

// TestConfigMapWatcherNoGoroutineLeak is a regression test for a goroutine leak
// that occurred when using apiwatch.Filter for the ConfigMap watch.
//
// The bug: apiwatch.Filter spawns a filteredWatch.loop() goroutine that reads events
// from the inner watcher and writes them to an unbuffered result channel. When the
// reflector's watchHandler exits via <-stopCh and stops consuming from the result
// channel, any in-flight event causes filteredWatch.loop() to block permanently on
// the unbuffered channel send with no cancellation path — a permanent goroutine leak.
// Each KSM check assignment added goroutines that were never released, causing the
// live heap to grow with accumulated decoded ConfigMap objects.
//
// The fix: configMapWatcher uses a context-aware forwarding goroutine that selects
// on ctx.Done() when sending events, so it exits cleanly when the context is cancelled
// even if nobody is consuming from ResultChan().
func TestConfigMapWatcherNoGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inner := apiwatch.NewRaceFreeFake()
	// We deliberately discard the watcher handle to avoid calling Stop() on it —
	// the test verifies that context cancellation alone is sufficient to exit the
	// loop goroutine even when nobody ever reads from ResultChan().
	newConfigMapWatcher(ctx, inner)

	// Send an event so the loop goroutine has a converted ConfigMap event
	// ready to forward to its result channel.
	inner.Add(&metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cm",
			Namespace: "default",
		},
	})

	// Intentionally do NOT read from the result channel. This simulates the race
	// where the reflector's watchHandler has already exited via <-stopCh while
	// the watcher goroutine still has an event to deliver to its unbuffered channel.
	//
	// With the old apiwatch.Filter implementation, the filteredWatch.loop() goroutine
	// would block here permanently on `fw.result <- event` with no exit path.
	// With configMapWatcher, the goroutine exits via ctx.Done() when we cancel below.
	cancel()

	// goleak.VerifyNone (deferred above) will poll until all goroutines from this
	// package have exited or the timeout elapses, catching any leaked goroutine.
}

func TestConfigMapWatcherConvertsPartialObjectMetadata(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inner := apiwatch.NewRaceFreeFake()
	cw := newConfigMapWatcher(ctx, inner)
	defer cw.Stop()

	inner.Add(&metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "my-config",
			Namespace:       "kube-system",
			UID:             "uid-42",
			ResourceVersion: "100",
		},
	})

	select {
	case event := <-cw.ResultChan():
		assert.Equal(t, apiwatch.Added, event.Type)
		cm, ok := event.Object.(*corev1.ConfigMap)
		require.True(t, ok, "expected event.Object to be *corev1.ConfigMap")
		assert.Equal(t, "my-config", cm.Name)
		assert.Equal(t, "kube-system", cm.Namespace)
		assert.Equal(t, "uid-42", string(cm.UID))
		assert.Equal(t, "100", cm.ResourceVersion)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for converted ConfigMap event")
	}
}

func TestConfigMapWatcherFiltersNonPartialObjects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inner := apiwatch.NewRaceFreeFake()
	cw := newConfigMapWatcher(ctx, inner)
	defer cw.Stop()

	// A raw ConfigMap (not PartialObjectMetadata) should be silently dropped.
	inner.Add(&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "should-be-filtered"}})

	// A valid PartialObjectMetadata event sent after should pass through.
	inner.Add(&metav1.PartialObjectMetadata{
		ObjectMeta: metav1.ObjectMeta{Name: "should-pass", Namespace: "ns"},
	})

	select {
	case event := <-cw.ResultChan():
		cm, ok := event.Object.(*corev1.ConfigMap)
		require.True(t, ok)
		assert.Equal(t, "should-pass", cm.Name, "filtered object should not appear; only valid PartialObjectMetadata should pass through")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestConfigMapWatcherStopsWhenInnerCloses(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	inner := apiwatch.NewRaceFreeFake()
	cw := newConfigMapWatcher(ctx, inner)

	// Stopping the inner watcher closes its result channel, which should
	// cause the loop goroutine to exit cleanly via the ok==false branch.
	inner.Stop()

	// Result channel should be closed shortly after the inner watcher stops.
	select {
	case _, ok := <-cw.ResultChan():
		assert.False(t, ok, "ResultChan should be closed when inner watcher stops")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for ResultChan to close after inner watcher stopped")
	}
}
