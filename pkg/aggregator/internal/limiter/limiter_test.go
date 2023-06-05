// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package limiter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLimiter(t *testing.T) {
	l := New(1, "pod", []string{"srv"})

	// check that:
	// - unrelated tags are not used
	// - tags without values are not used
	// - missing tag maps to a the same identity

	a := assert.New(t)

	a.Equal(l.telemetryTagNames, []string{"srv:", "pod:"})

	a.True(l.Track([]string{"srv:foo", "cid:1", "pod", "pod:foo"}))
	a.False(l.Track([]string{"srv:foo", "cid:2", "pod", "pod:foo"}))

	a.True(l.Track([]string{"srv:foo", "cid:3", "pod", "pod:bar"}))
	a.False(l.Track([]string{"srv:foo", "cid:4", "pod", "pod:bar"}))

	a.True(l.Track([]string{"srv:foo", "cid:5", "pod"}))
	a.False(l.Track([]string{"srv:foo", "cid:6", "pod"}))
	a.False(l.Track([]string{}))

	l.Remove([]string{})
	a.True(l.Track([]string{}))

	l.Remove([]string{"srv:bar", "pod:foo"})
	a.True(l.Track([]string{"srv:bar", "pod:foo"}))

	a.Equal(&entry{
		current:       1,
		rejected:      0,
		telemetryTags: []string{"srv:bar", "pod:foo"},
	}, l.usage["pod:foo"])

	l.Remove([]string{"pod:foo"})
	a.Nil(l.usage["pod:foo"])
}

func TestGlobal(t *testing.T) {
	l := NewGlobal(2, 1, "pod", []string{})
	a := assert.New(t)

	a.True(l.Track([]string{"pod:foo"}))
	a.True(l.Track([]string{"pod:foo"}))
	a.False(l.Track([]string{"pod:foo"}))
	a.False(l.Track([]string{"pod:bar"})) // would exceed global limit

	l.Remove([]string{"pod:foo"})

	a.False(l.Track([]string{"pod:foo"})) // would exceed per-origin limit

	a.True(l.Track([]string{"pod:bar"}))
	a.False(l.Track([]string{"pod:bar"})) // would exceed per-origin limit

	l.Remove([]string{"pod:bar"}) // removes origin entry, limit is 2 again
	a.True(l.Track([]string{"pod:foo"}))

	// check for division by zero
	l.Remove([]string{"pod:foo"})
	l.Remove([]string{"pod:foo"})
	a.Equal(0, len(l.usage))
}

func TestExpire(t *testing.T) {
	l := NewGlobal(2, 1, "pod", []string{})
	a := assert.New(t)

	foo := []string{"pod:foo"}
	bar := []string{"pod:bar"}

	a.True(l.Track(foo))
	a.True(l.Track(foo))
	a.False(l.Track(bar)) // rejected, but allocates limit to bar

	l.ExpireEntries()

	l.Remove(foo)
	// maxAge 1 means limit remains reserved for 1 tick after initial sample
	a.False(l.Track(foo))
	a.Len(l.usage, 2)

	l.ExpireEntries()

	a.Len(l.usage, 1)
	l.Remove([]string{"pod:foo"})
	a.True(l.Track([]string{"pod:foo"}))
}
