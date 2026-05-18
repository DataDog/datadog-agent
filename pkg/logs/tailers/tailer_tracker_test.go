// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package tailers

import (
	"testing"

	assert "github.com/stretchr/testify/require"

	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

type TestTailer1 struct {
	id   string
	info *status.InfoRegistry
}

func NewTestTailer1(id string) *TestTailer1 {
	return &TestTailer1{
		id:   id,
		info: status.NewInfoRegistry(),
	}
}

func (t *TestTailer1) GetID() string {
	return t.id
}
func (t *TestTailer1) GetType() string {
	return "test"
}
func (t *TestTailer1) GetInfo() *status.InfoRegistry {
	return t.info
}

type TestTailer2 struct {
	id   string
	info *status.InfoRegistry
}

func NewTestTailer2(id string) *TestTailer2 {
	return &TestTailer2{
		id:   id,
		info: status.NewInfoRegistry(),
	}
}

func (t *TestTailer2) GetID() string {
	return t.id
}

func (t *TestTailer2) GetType() string {
	return "test"
}

func (t *TestTailer2) GetInfo() *status.InfoRegistry {
	return t.info
}

func TestCollectAllTailers(t *testing.T) {

	container1 := NewTailerContainer[*TestTailer1]()
	container1.Add(NewTestTailer1("1a"))
	t1b := NewTestTailer1("1b")
	container1.Add(t1b)

	container2 := NewTailerContainer[*TestTailer2]()
	container2.Add(NewTestTailer2("2a"))
	container2.Add(NewTestTailer2("2b"))

	tracker := NewTailerTracker()
	tracker.Add(container1)
	tracker.Add(container2)

	tailers := tracker.All()
	assert.Equal(t, 4, len(tailers))

	results := make(map[string]bool)
	for _, t := range tailers {
		results[t.GetID()] = true
	}

	for _, k := range []string{"1a", "1b", "2a", "2b"} {
		assert.True(t, results[k])
	}

	container1.Remove(t1b)

	results = make(map[string]bool)
	for _, t := range tailers {
		results[t.GetID()] = true
	}

	for _, k := range []string{"1a", "2a", "2b"} {
		assert.True(t, results[k])
	}

}

// TestDuplicateTailerIDsAreAllPreserved is a regression test for AGNTLOG-317.
// When two tailers share the same GetID() (for example, two log configurations
// accidentally tailing the same file path) both must remain observable from
// the TailerTracker so the agent status surfaces every active tailer.
func TestDuplicateTailerIDsAreAllPreserved(t *testing.T) {
	container := NewTailerContainer[*TestTailer1]()

	t1 := NewTestTailer1("/var/log/app.log")
	t2 := NewTestTailer1("/var/log/app.log")
	t3 := NewTestTailer1("/var/log/other.log")

	container.Add(t1)
	container.Add(t2)
	container.Add(t3)

	// Count and All must reflect every instance even when IDs collide.
	assert.Equal(t, 3, container.Count())
	assert.Equal(t, 3, len(container.All()))

	// The status path consumes the tracker view; it must surface both
	// duplicate-ID tailers, not just one of them.
	tracker := NewTailerTracker()
	tracker.Add(container)

	tailers := tracker.All()
	assert.Equal(t, 3, len(tailers))

	seen := make(map[*TestTailer1]bool)
	for _, item := range tailers {
		tt, ok := item.(*TestTailer1)
		assert.True(t, ok)
		seen[tt] = true
	}
	assert.True(t, seen[t1], "first duplicate-id tailer must remain visible")
	assert.True(t, seen[t2], "second duplicate-id tailer must remain visible")
	assert.True(t, seen[t3])

	// Contains is true while at least one duplicate-id tailer remains.
	assert.True(t, container.Contains("/var/log/app.log"))

	// Get returns one of the duplicates (the implementation returns the
	// first one registered).
	got, ok := container.Get("/var/log/app.log")
	assert.True(t, ok)
	assert.Equal(t, t1, got)

	// Removing one duplicate keeps the other discoverable; Contains stays true.
	container.Remove(t1)
	assert.True(t, container.Contains("/var/log/app.log"))
	assert.Equal(t, 2, container.Count())
	got, ok = container.Get("/var/log/app.log")
	assert.True(t, ok)
	assert.Equal(t, t2, got)

	// Removing the last duplicate clears the entry entirely.
	container.Remove(t2)
	assert.False(t, container.Contains("/var/log/app.log"))
	_, ok = container.Get("/var/log/app.log")
	assert.False(t, ok)
	assert.Equal(t, 1, container.Count())
}
