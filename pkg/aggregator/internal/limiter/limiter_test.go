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

	a.Equal(l.tags, []string{"srv:", "pod:"})

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
		current:  1,
		accepted: 1,
		rejected: 0,
		tags:     []string{"srv:bar", "pod:foo"},
	}, l.usage["pod:foo"])

	l.Remove([]string{"pod:foo"})
	a.Nil(l.usage["pod:foo"])
}
