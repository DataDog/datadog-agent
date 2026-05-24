// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package eventbuf

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
)

// testBudget is a Budget with effectively no ceiling, for tests that
// don't want to think about budget behavior. Use newTestBuffer when no
// budget-specific behavior is under test.
const testBudgetBytes = 1 << 30

// newTestBuffer returns a Buffer with an effectively-unlimited budget.
// Use this when the test doesn't care about the byte ceiling.
func newTestBuffer() *Buffer {
	return NewBuffer(NewBudget(testBudgetBytes))
}

// testMessage is a minimal Message implementation for unit tests.
type testMessage struct {
	data     []byte
	released bool
}

func newTestMessage(size int) *testMessage {
	return &testMessage{data: make([]byte, size)}
}

func (m *testMessage) Event() output.Event { return output.Event(m.data) }
func (m *testMessage) Release()            { m.released = true }

func TestMessageListAppendIterRelease(t *testing.T) {
	m1 := newTestMessage(8)
	m2 := newTestMessage(16)
	m3 := newTestMessage(24)

	l := NewMessageList(m1)
	l.Append(m2)
	l.Append(m3)

	var got []int
	for ev := range l.Fragments() {
		got = append(got, len(ev))
	}
	assert.Equal(t, []int{8, 16, 24}, got)
	assert.Equal(t, 48, l.TotalSize())
	assert.Equal(t, 8, len(l.Head()))

	l.Release()
	assert.True(t, m1.released)
	assert.True(t, m2.released)
	assert.True(t, m3.released)
}

func TestMessageListSingle(t *testing.T) {
	m := newTestMessage(8)
	l := NewMessageList(m)
	require.Equal(t, 8, l.TotalSize())

	count := 0
	for range l.Fragments() {
		count++
	}
	assert.Equal(t, 1, count)

	l.Release()
	assert.True(t, m.released)
}
