// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

//go:build kubeapiserver

package apiserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func makeStore() (*eventReflectorStore, *[]*v1.Event) {
	var captured []*v1.Event
	s := &eventReflectorStore{
		enqueue: func(obj interface{}) {
			if ev, ok := obj.(*v1.Event); ok {
				captured = append(captured, ev)
			}
		},
	}
	return s, &captured
}

func eventWithRV(rv string) *v1.Event {
	return &v1.Event{ObjectMeta: metav1.ObjectMeta{ResourceVersion: rv}}
}

// TestParseResourceVersion verifies valid, empty, and non-numeric inputs.
func TestParseResourceVersion(t *testing.T) {
	for _, tc := range []struct {
		name string
		rv   string
		want uint64
	}{
		{"empty string returns zero", "", 0},
		{"valid uint is parsed", "42", 42},
		{"non-numeric returns zero", "abc", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, parseResourceVersion(tc.rv))
		})
	}
}

// TestForwardIfNew verifies the monotonic-RV gate: non-Events and stale RVs are dropped; new RVs are forwarded.
func TestForwardIfNew(t *testing.T) {
	t.Run("non-Event object is dropped", func(t *testing.T) {
		s, captured := makeStore()
		s.forwardIfNew("not an event")
		assert.Empty(t, *captured)
	})

	t.Run("RV at or below highestRV is not forwarded", func(t *testing.T) {
		s, captured := makeStore()
		s.highestRV = 10
		s.forwardIfNew(eventWithRV("10")) // boundary: equal
		assert.Empty(t, *captured)
		assert.Equal(t, uint64(10), s.highestRV)
	})

	t.Run("RV above highestRV is forwarded and highestRV advances", func(t *testing.T) {
		s, captured := makeStore()
		s.highestRV = 10
		ev := eventWithRV("20")
		s.forwardIfNew(ev)
		require.Len(t, *captured, 1)
		assert.Same(t, ev, (*captured)[0])
		assert.Equal(t, uint64(20), s.highestRV)
	})
}

// TestAddUpdate verifies Add and Update both delegate to forwardIfNew.
func TestAddUpdate(t *testing.T) {
	methods := []struct {
		name string
		call func(*eventReflectorStore, interface{}) error
	}{
		{"Add", (*eventReflectorStore).Add},
		{"Update", (*eventReflectorStore).Update},
	}

	t.Run("new RV: event forwarded", func(t *testing.T) {
		for _, m := range methods {
			s, captured := makeStore()
			ev := eventWithRV("1")
			require.NoError(t, m.call(s, ev))
			require.Len(t, *captured, 1, m.name)
			assert.Same(t, ev, (*captured)[0], m.name)
		}
	})

	t.Run("stale RV: event skipped", func(t *testing.T) {
		for _, m := range methods {
			s, captured := makeStore()
			s.highestRV = 5
			require.NoError(t, m.call(s, eventWithRV("3")))
			assert.Empty(t, *captured, m.name)
		}
	})
}

// TestDelete verifies Delete is a no-op.
func TestDelete(t *testing.T) {
	s, captured := makeStore()
	require.NoError(t, s.Delete(eventWithRV("99")))
	assert.Empty(t, *captured)
}

// TestReplace verifies first-call seeding suppresses forwarding, and subsequent calls forward qualifying events without lowering highestRV.
func TestReplace(t *testing.T) {
	t.Run("first call seeds highestRV from listRV and skips all events", func(t *testing.T) {
		s, captured := makeStore()
		require.NoError(t, s.Replace([]interface{}{eventWithRV("5")}, "10"))
		assert.Empty(t, *captured)
		assert.True(t, s.seeded)
		assert.Equal(t, uint64(10), s.highestRV)
	})

	t.Run("subsequent call forwards new events and does not lower highestRV below its current value", func(t *testing.T) {
		s, captured := makeStore()
		require.NoError(t, s.Replace([]interface{}{}, "5")) // seed; highestRV=5
		ev := eventWithRV("10")
		// listRV "3" is below highestRV after forwarding ev; highestRV must not drop.
		require.NoError(t, s.Replace([]interface{}{ev, eventWithRV("3")}, "3"))
		require.Len(t, *captured, 1)
		assert.Same(t, ev, (*captured)[0])
		assert.Equal(t, uint64(10), s.highestRV)
	})
}

// TestNoOpMethods verifies unimplemented cache.Store methods return zero values.
func TestNoOpMethods(t *testing.T) {
	s, _ := makeStore()
	assert.Nil(t, s.List())
	assert.Nil(t, s.ListKeys())
	item, exists, err := s.Get(nil)
	assert.Nil(t, item)
	assert.False(t, exists)
	assert.NoError(t, err)
	item, exists, err = s.GetByKey("")
	assert.Nil(t, item)
	assert.False(t, exists)
	assert.NoError(t, err)
	assert.NoError(t, s.Resync())
}
