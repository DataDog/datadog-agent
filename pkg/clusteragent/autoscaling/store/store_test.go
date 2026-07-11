// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package store_test exercises Store through its public interface.
package store_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/autoscaling/store"
)

// testItem includes a reference-typed field to exercise shallow-copy behavior.
type testItem struct {
	name  string
	value int
	tags  []string
}

// Store must satisfy the Observable interface.
var _ store.Observable = (*store.Store[testItem])(nil)

const (
	// blockWindow bounds negative assertions that expect an operation to block.
	blockWindow = 100 * time.Millisecond
	// settleWindow bounds operations that should complete.
	settleWindow = 2 * time.Second
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func recvWithin[V any](t *testing.T, ch <-chan V, d time.Duration, msg string) V {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(d):
		t.Fatalf("timed out (%s): %s", d, msg)
		var zero V
		return zero
	}
}

func notWithin[V any](t *testing.T, ch <-chan V, d time.Duration, msg string) {
	t.Helper()
	select {
	case <-ch:
		t.Fatalf("received unexpectedly within %s: %s", d, msg)
	case <-time.After(d):
	}
}

// recordingObserver captures every notification with its sender, safe for concurrent use.
type recordingObserver struct {
	mu      sync.Mutex
	sets    []notification
	deletes []notification
}

type notification struct {
	key    string
	sender store.SenderID
}

func (r *recordingObserver) observer() store.Observer {
	return store.Observer{
		SetFunc: func(key string, sender store.SenderID) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.sets = append(r.sets, notification{key, sender})
		},
		DeleteFunc: func(key string, sender store.SenderID) {
			r.mu.Lock()
			defer r.mu.Unlock()
			r.deletes = append(r.deletes, notification{key, sender})
		},
	}
}

func (r *recordingObserver) setEvents() []notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]notification(nil), r.sets...)
}

func (r *recordingObserver) deleteEvents() []notification {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]notification(nil), r.deletes...)
}

// set and del arrange store state through public Get handles.
func set(s *store.Store[testItem], id string, v testItem, sender store.SenderID) {
	item, _ := s.Get(id)
	item.Upsert(v, sender)
}

func del(s *store.Store[testItem], id string, sender store.SenderID) {
	item, found := s.Get(id)
	if !found {
		item.Release()
		return
	}
	item.Delete(sender)
}

// ---------------------------------------------------------------------------
// Basic key/value behavior
// ---------------------------------------------------------------------------

func TestPeekMissing(t *testing.T) {
	s := store.NewStore[testItem]()
	_, found := s.Peek("absent")
	assert.False(t, found)
}

func TestCount(t *testing.T) {
	s := store.NewStore[testItem]()
	assert.Equal(t, 0, s.Count())
	set(s, "a", testItem{}, "test")
	set(s, "b", testItem{}, "test")
	assert.Equal(t, 2, s.Count())
	del(s, "a", "test")
	assert.Equal(t, 1, s.Count())
}

func TestListAll(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "a", testItem{name: "a"}, "test")
	set(s, "b", testItem{name: "b"}, "test")

	all := s.List(nil)
	assert.Len(t, all, 2)
	names := map[string]struct{}{}
	for _, it := range all {
		names[it.name] = struct{}{}
	}
	assert.Equal(t, map[string]struct{}{"a": {}, "b": {}}, names)
}

func TestListFiltered(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "a", testItem{name: "a", value: 1}, "test")
	set(s, "b", testItem{name: "b", value: 2}, "test")
	set(s, "c", testItem{name: "c", value: 3}, "test")

	filtered := s.List(func(it testItem) bool { return it.value >= 2 })
	assert.Len(t, filtered, 2)
	for _, it := range filtered {
		assert.GreaterOrEqual(t, it.value, 2)
	}
}

// TestListFilterMayReentrantlyReadStore verifies filters run without the store lock.
func TestListFilterMayReentrantlyReadStore(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "a", testItem{name: "a", value: 1}, "test")
	set(s, "b", testItem{name: "b", value: 2}, "test")

	done := make(chan []testItem, 1)
	go func() {
		done <- s.List(func(it testItem) bool {
			_, ok := s.Peek(it.name)
			return ok && it.value >= 2
		})
	}()

	got := recvWithin(t, done, settleWindow, "List deadlocked when its filter re-entered the store")
	require.Len(t, got, 1)
	assert.Equal(t, "b", got[0].name)
}

// TestListReturnsTopLevelCopies verifies returned values are top-level copies.
func TestListReturnsTopLevelCopies(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "a", testItem{name: "a", value: 1}, "test")

	got := s.List(nil)
	for i := range got {
		got[i].value = 999
	}

	stored, _ := s.Peek("a")
	assert.Equal(t, 1, stored.value, "mutating a top-level field of the List result must not change the store")
}

// ---------------------------------------------------------------------------
// Observers
// ---------------------------------------------------------------------------

func TestMultipleObservers(t *testing.T) {
	s := store.NewStore[testItem]()
	ro1, ro2 := &recordingObserver{}, &recordingObserver{}
	s.RegisterObserver(ro1.observer())
	s.RegisterObserver(ro2.observer())

	set(s, "a", testItem{}, "sender-x")
	assert.Len(t, ro1.setEvents(), 1)
	assert.Len(t, ro2.setEvents(), 1)
}

// TestNilObserverFuncsIgnored: a partially-populated Observer must not panic.
func TestNilObserverFuncsIgnored(t *testing.T) {
	s := store.NewStore[testItem]()
	var deletes int
	s.RegisterObserver(store.Observer{
		DeleteFunc: func(string, store.SenderID) { deletes++ },
	}) // SetFunc is nil

	assert.NotPanics(t, func() {
		set(s, "a", testItem{}, "x") // no SetFunc registered
		del(s, "a", "x")
	})
	assert.Equal(t, 1, deletes)
}

// ---------------------------------------------------------------------------
// Get-and-lock handle semantics
// ---------------------------------------------------------------------------

func TestGetExistingValue(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "a", testItem{name: "a", value: 7}, "test")

	item, found := s.Get("a")
	require.True(t, found)
	assert.Equal(t, 7, item.Value().value)
	item.Release()
}

func TestGetUpsert(t *testing.T) {
	s := store.NewStore[testItem]()
	ro := &recordingObserver{}
	s.RegisterObserver(ro.observer())
	set(s, "a", testItem{value: 1}, "init")

	item, _ := s.Get("a")
	v := item.Value()
	v.value = 42
	item.Upsert(v, "upserter")

	got, _ := s.Peek("a")
	assert.Equal(t, 42, got.value)
	// One set event for the init, one for the upsert.
	assert.Equal(t, []notification{{"a", "init"}, {"a", "upserter"}}, ro.setEvents())
}

// TestGetMissingThenUpsertCreates verifies a locked missing key can be created.
func TestGetMissingThenUpsertCreates(t *testing.T) {
	s := store.NewStore[testItem]()

	item, found := s.Get("new")
	require.False(t, found)
	assert.Equal(t, testItem{}, item.Value())
	item.Upsert(testItem{name: "new", value: 5}, "creator")

	got, found := s.Peek("new")
	require.True(t, found)
	assert.Equal(t, 5, got.value)
}

func TestGetDelete(t *testing.T) {
	s := store.NewStore[testItem]()
	ro := &recordingObserver{}
	s.RegisterObserver(ro.observer())
	set(s, "a", testItem{value: 1}, "init")

	item, found := s.Get("a")
	require.True(t, found)
	item.Delete("deleter")

	_, found = s.Peek("a")
	assert.False(t, found)
	assert.Equal(t, []notification{{"a", "deleter"}}, ro.deleteEvents())
}

// TestGetMissingDeleteDoesNotNotify: deleting a not-found locked key is a no-op.
func TestGetMissingDeleteDoesNotNotify(t *testing.T) {
	s := store.NewStore[testItem]()
	ro := &recordingObserver{}
	s.RegisterObserver(ro.observer())

	item, found := s.Get("absent")
	require.False(t, found)
	item.Delete("deleter")

	assert.Empty(t, ro.deleteEvents())
}

func TestGetReleaseMakesNoChange(t *testing.T) {
	s := store.NewStore[testItem]()
	ro := &recordingObserver{}
	s.RegisterObserver(ro.observer())
	set(s, "a", testItem{value: 1}, "init")
	ro2 := &recordingObserver{} // observe only the discard window
	s.RegisterObserver(ro2.observer())

	item, _ := s.Get("a")
	v := item.Value()
	v.value = 999
	item.Release() // value 999 is thrown away

	got, _ := s.Peek("a")
	assert.Equal(t, 1, got.value)
	assert.Empty(t, ro2.setEvents(), "Release must not notify")
	assert.Empty(t, ro2.deleteEvents(), "Release must not notify")
}

// TestDoubleTerminalIsNoop verifies only the first terminal call takes effect.
func TestDoubleTerminalIsNoop(t *testing.T) {
	s := store.NewStore[testItem]()
	ro := &recordingObserver{}
	s.RegisterObserver(ro.observer())
	set(s, "a", testItem{value: 1}, "init")

	item, _ := s.Get("a")
	v := item.Value()
	v.value = 5
	item.Upsert(v, "upserter") // wins
	item.Release()             // no-op
	item.Delete("deleter")     // no-op
	item.Upsert(v, "again")    // no-op

	got, _ := s.Peek("a")
	assert.Equal(t, 5, got.value)
	// Exactly the init + the single winning commit; no delete.
	assert.Equal(t, []notification{{"a", "init"}, {"a", "upserter"}}, ro.setEvents())
	assert.Empty(t, ro.deleteEvents())
}

// TestDeferReleaseAfterUpsert verifies deferred Release does not undo Upsert.
func TestDeferReleaseAfterUpsert(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "a", testItem{value: 1}, "init")

	func() {
		item, _ := s.Get("a")
		defer item.Release()
		v := item.Value()
		v.value = 9
		item.Upsert(v, "upserter")
	}()

	got, _ := s.Peek("a")
	assert.Equal(t, 9, got.value)
}

// TestGetSkipIfAbsent verifies Get on a missing key does not leak its lock.
func TestGetSkipIfAbsent(t *testing.T) {
	s := store.NewStore[testItem]()

	item, found := s.Get("absent")
	assert.False(t, found)
	item.Release()

	done := make(chan struct{})
	go func() {
		set(s, "absent", testItem{value: 1}, "x")
		close(done)
	}()
	recvWithin(t, done, settleWindow, "store wedged after Get+Release on an absent key")
}

// ---------------------------------------------------------------------------
// Concurrency properties
// ---------------------------------------------------------------------------

// TestSameKeyMutualExclusion verifies same-key lockers run one at a time.
func TestSameKeyMutualExclusion(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "k", testItem{value: 1}, "init")

	entered := make(chan struct{})
	proceed := make(chan struct{})
	go func() {
		item, _ := s.Get("k")
		close(entered)
		<-proceed
		v := item.Value()
		v.value = 2
		item.Upsert(v, "g1")
	}()
	<-entered // g1 holds k's lock

	g2 := make(chan int)
	go func() {
		item, _ := s.Get("k")
		got := item.Value().value
		item.Release()
		g2 <- got
	}()

	notWithin(t, g2, blockWindow, "second Get(k) must block while the first holds it")
	close(proceed)
	got := recvWithin(t, g2, settleWindow, "second Get(k) must proceed once the first releases")
	assert.Equal(t, 2, got, "second locker must observe the first's committed value")
}

// TestDifferentKeysAreConcurrent verifies one held key does not block another key.
func TestDifferentKeysAreConcurrent(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "k1", testItem{value: 1}, "init")
	set(s, "k2", testItem{value: 2}, "init")

	entered := make(chan struct{})
	proceed := make(chan struct{})
	go func() {
		item, _ := s.Get("k1")
		close(entered)
		<-proceed
		item.Release()
	}()
	<-entered // k1 is held

	done := make(chan struct{})
	go func() {
		item, _ := s.Get("k2")
		item.Release()
		close(done)
	}()
	recvWithin(t, done, settleWindow, "locking k2 must not block on held k1")
	close(proceed)
}

// TestReadsDoNotBlockBehindHeldItem verifies reads see the last committed value.
func TestReadsDoNotBlockBehindHeldItem(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "k1", testItem{value: 7}, "init")
	set(s, "k2", testItem{value: 8}, "init")

	entered := make(chan struct{})
	proceed := make(chan struct{})
	go func() {
		item, _ := s.Get("k1")
		close(entered)
		<-proceed
		v := item.Value()
		v.value = 99
		item.Upsert(v, "g1")
	}()
	<-entered // k1 is held, not yet committed

	done := make(chan int)
	go func() {
		v, _ := s.Peek("k1")
		_ = s.Count()
		_ = s.List(nil)
		done <- v.value
	}()
	got := recvWithin(t, done, settleWindow, "reads must not block behind a held item")
	assert.Equal(t, 7, got, "Get during a held-but-uncommitted item must see the last committed value")
	close(proceed)
}

// TestObserverReentrancyFromSet verifies set observers run after item unlock.
func TestObserverReentrancyFromSet(t *testing.T) {
	s := store.NewStore[testItem]()
	var reAcquired atomic.Bool
	var seen atomic.Int64
	s.RegisterObserver(store.Observer{
		SetFunc: func(key string, _ store.SenderID) {
			if v, ok := s.Peek(key); ok {
				seen.Store(int64(v.value))
			}
			item, _ := s.Get(key)
			item.Release()
			reAcquired.Store(true)
		},
	})

	done := make(chan struct{})
	go func() {
		set(s, "k", testItem{value: 42}, "x")
		close(done)
	}()
	recvWithin(t, done, settleWindow, "Set deadlocked when its observer re-entered the store")
	assert.True(t, reAcquired.Load(), "observer must be able to re-acquire the released lock")
	assert.Equal(t, int64(42), seen.Load(), "observer must see the committed value")
}

// TestObserverReentrancyFromUpsert verifies handle Upsert has the same observer ordering.
func TestObserverReentrancyFromUpsert(t *testing.T) {
	s := store.NewStore[testItem]()
	var reAcquired atomic.Bool
	s.RegisterObserver(store.Observer{
		SetFunc: func(key string, _ store.SenderID) {
			item, _ := s.Get(key)
			item.Release()
			reAcquired.Store(true)
		},
	})

	done := make(chan struct{})
	go func() {
		item, _ := s.Get("k")
		item.Upsert(testItem{value: 7}, "x")
		close(done)
	}()
	recvWithin(t, done, settleWindow, "Upsert deadlocked when its observer re-entered the store")
	assert.True(t, reAcquired.Load())
}

// TestDeleteObserverReentrancy verifies delete observers run after item unlock.
func TestDeleteObserverReentrancy(t *testing.T) {
	s := store.NewStore[testItem]()
	var reAcquired atomic.Bool
	s.RegisterObserver(store.Observer{
		DeleteFunc: func(key string, _ store.SenderID) {
			item, _ := s.Get(key)
			item.Release()
			reAcquired.Store(true)
		},
	})
	set(s, "k", testItem{value: 1}, "x")

	done := make(chan struct{})
	go func() {
		del(s, "k", "x")
		close(done)
	}()
	recvWithin(t, done, settleWindow, "Delete deadlocked when its observer re-entered the store")
	assert.True(t, reAcquired.Load())
}

// TestHandleDeleteObserverReentrancy verifies LockedItem.Delete observer ordering.
func TestHandleDeleteObserverReentrancy(t *testing.T) {
	s := store.NewStore[testItem]()
	var reAcquired atomic.Bool
	s.RegisterObserver(store.Observer{
		DeleteFunc: func(key string, _ store.SenderID) {
			item, _ := s.Get(key)
			item.Release()
			reAcquired.Store(true)
		},
	})
	set(s, "k", testItem{value: 1}, "x")

	done := make(chan struct{})
	go func() {
		item, _ := s.Get("k")
		item.Delete("x")
		close(done)
	}()
	recvWithin(t, done, settleWindow, "LockedItem.Delete deadlocked when its observer re-entered the store")
	assert.True(t, reAcquired.Load())
}

// TestProcessAllObserverReentrancy verifies ProcessAll notifies after item unlock.
func TestProcessAllObserverReentrancy(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "setk", testItem{value: 1}, "x")
	set(s, "delk", testItem{value: 2}, "x")

	var reAcquired atomic.Int64
	reenter := func(key string, _ store.SenderID) {
		item, _ := s.Get(key)
		item.Release()
		reAcquired.Add(1)
	}
	s.RegisterObserver(store.Observer{SetFunc: reenter, DeleteFunc: reenter})

	done := make(chan struct{})
	go func() {
		s.ProcessAll("pa", func(id string, v testItem) (testItem, store.ItemAction) {
			if id == "setk" {
				v.value = 10
				return v, store.SetItem
			}
			return v, store.DeleteItem
		})
		close(done)
	}()
	recvWithin(t, done, settleWindow, "ProcessAll deadlocked when its observers re-entered the store")
	assert.Equal(t, int64(2), reAcquired.Load())
}

// TestReferenceFieldReplaceContract verifies reference fields are changed by replacement.
func TestReferenceFieldReplaceContract(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "a", testItem{tags: []string{"orig"}}, "init")

	item, _ := s.Get("a")
	v := item.Value()
	v.tags = []string{"committed"}
	item.Upsert(v, "upserter")
	got, _ := s.Peek("a")
	assert.Equal(t, []string{"committed"}, got.tags)

	item2, _ := s.Get("a")
	v2 := item2.Value()
	v2.tags = []string{"discarded"}
	item2.Release()
	got2, _ := s.Peek("a")
	assert.Equal(t, []string{"committed"}, got2.tags, "a replaced-but-discarded field must not persist")
}

// TestValueAfterTerminalIsZero verifies consumed handles are inert.
func TestValueAfterTerminalIsZero(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "a", testItem{value: 5}, "init")

	item, _ := s.Get("a")
	item.Upsert(testItem{value: 6}, "upserter")
	assert.Equal(t, testItem{}, item.Value(), "Value() on a consumed handle must return the zero value")
}

// TestConcurrentIncrementsNoLostUpdates verifies same-key writes are serialized.
func TestConcurrentIncrementsNoLostUpdates(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "k", testItem{value: 0}, "init")

	const goroutines, iterations = 8, 250
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				item, _ := s.Get("k")
				v := item.Value()
				v.value++
				item.Upsert(v, "worker")
			}
		}()
	}
	wg.Wait()

	got, _ := s.Peek("k")
	assert.Equal(t, goroutines*iterations, got.value, "lost updates indicate broken mutual exclusion")
}

// TestConcurrentChurnNoCorruption stresses lock registry churn. Run with -race.
func TestConcurrentChurnNoCorruption(t *testing.T) {
	s := store.NewStore[testItem]()
	keys := []string{"a", "b", "c", "d"}

	const goroutines, iterations = 16, 400
	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				k := keys[(seed+j)%len(keys)]
				switch j % 6 {
				case 0:
					set(s, k, testItem{value: j}, "s")
				case 1:
					del(s, k, "d")
				case 2:
					item, found := s.Get(k)
					if found {
						v := item.Value()
						v.value++
						item.Upsert(v, "u")
					} else {
						item.Upsert(testItem{value: 1}, "u") // create
					}
				case 3:
					item, found := s.Get(k)
					if found {
						item.Delete("d2")
					} else {
						item.Release()
					}
				case 4:
					_ = s.List(func(testItem) bool { return true })
					_, _ = s.Peek(k)
					_ = s.Count()
				case 5:
					s.ProcessAll("pa", func(_ string, v testItem) (testItem, store.ItemAction) {
						if v.value%2 == 0 {
							v.value++
							return v, store.SetItem
						}
						return v, store.KeepItem
					})
				}
			}
		}(i)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	recvWithin(t, done, 30*time.Second, "concurrent churn deadlocked")
}

// ---------------------------------------------------------------------------
// ProcessAll
// ---------------------------------------------------------------------------

func TestProcessAllActions(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "keep", testItem{value: 1}, "init")
	set(s, "set", testItem{value: 2}, "init")
	set(s, "del", testItem{value: 3}, "init")

	s.ProcessAll("pa", func(id string, v testItem) (testItem, store.ItemAction) {
		switch id {
		case "set":
			v.value = 20
			return v, store.SetItem
		case "del":
			return v, store.DeleteItem
		default:
			return v, store.KeepItem
		}
	})

	keep, _ := s.Peek("keep")
	assert.Equal(t, 1, keep.value)
	set, _ := s.Peek("set")
	assert.Equal(t, 20, set.value)
	_, found := s.Peek("del")
	assert.False(t, found)
	assert.Equal(t, 2, s.Count())
}

func TestProcessAllNotifies(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "set", testItem{value: 2}, "init")
	set(s, "del", testItem{value: 3}, "init")
	ro := &recordingObserver{}
	s.RegisterObserver(ro.observer())

	s.ProcessAll("pa", func(id string, v testItem) (testItem, store.ItemAction) {
		if id == "set" {
			v.value = 20
			return v, store.SetItem
		}
		return v, store.DeleteItem
	})

	assert.Equal(t, []notification{{"set", "pa"}}, ro.setEvents())
	assert.Equal(t, []notification{{"del", "pa"}}, ro.deleteEvents())
}

// TestProcessAllSeesCurrentValue verifies processors see the current stored value.
func TestProcessAllSeesCurrentValue(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "k", testItem{value: 5}, "init")

	var seen int
	s.ProcessAll("pa", func(_ string, v testItem) (testItem, store.ItemAction) {
		seen = v.value
		return v, store.KeepItem
	})
	assert.Equal(t, 5, seen)
}

// TestProcessAllBlocksOnHeldKey verifies ProcessAll waits for held keys.
func TestProcessAllBlocksOnHeldKey(t *testing.T) {
	s := store.NewStore[testItem]()
	set(s, "k", testItem{value: 1}, "init")

	entered := make(chan struct{})
	proceed := make(chan struct{})
	go func() {
		item, _ := s.Get("k")
		close(entered)
		<-proceed
		item.Release()
	}()
	<-entered // k is held

	processed := make(chan struct{})
	go func() {
		s.ProcessAll("pa", func(_ string, v testItem) (testItem, store.ItemAction) {
			return v, store.KeepItem
		})
		close(processed)
	}()

	notWithin(t, processed, blockWindow, "ProcessAll must block on the held key")
	close(proceed)
	recvWithin(t, processed, settleWindow, "ProcessAll must complete once the key is released")
}

// TestProcessAllConcurrentSafe runs ProcessAll alongside concurrent writers. Run with -race.
func TestProcessAllConcurrentSafe(t *testing.T) {
	s := store.NewStore[testItem]()
	for _, k := range []string{"a", "b", "c"} {
		set(s, k, testItem{value: 0}, "init")
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			s.ProcessAll("pa", func(_ string, v testItem) (testItem, store.ItemAction) {
				v.value++
				return v, store.SetItem
			})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			item, _ := s.Get("a")
			v := item.Value()
			v.value++
			item.Upsert(v, "w")
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = s.List(nil)
			_, _ = s.Peek("b")
		}
	}()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	recvWithin(t, done, 30*time.Second, "ProcessAll concurrent with writers deadlocked")
}
