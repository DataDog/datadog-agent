// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package common

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewStringSet(t *testing.T) {
	s := NewStringSet()
	assert.NotNil(t, s)
	assert.Len(t, s, 0)

	s = NewStringSet("a", "b", "b", "c")
	assert.NotNil(t, s)
	assert.Len(t, s, 3)
}

func TestStringSetAdd(t *testing.T) {
	s := NewStringSet()
	s.Add("a")
	assert.Equal(t, []string{"a"}, s.GetAll())
	s.Add("b")
	res := sort.StringSlice(s.GetAll())
	res.Sort()
	assert.Equal(t, []string{"a", "b"}, []string(res))

	s.Add("b")
	res = sort.StringSlice(s.GetAll())
	res.Sort()
	assert.Equal(t, []string{"a", "b"}, []string(res))
}

func TestStringSetGetAll(t *testing.T) {
	s := NewStringSet("a", "b", "b", "c", "c")
	res := sort.StringSlice(s.GetAll())
	res.Sort()
	assert.Equal(t, []string{"a", "b", "c"}, []string(res))
}
