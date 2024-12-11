// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fxutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type dummyFunc func()

type dummyInterface interface {
	test()
}

type dummy1 struct{}

func (d *dummy1) test() {}

type dummy2 struct{}

func (d *dummy2) test() {}

type dummy3 struct{}

func (d dummy3) test() {}

func TestGetAndFilterGroup(t *testing.T) {
	// We don't want to filter "zero" version of basic types
	assert.Equal(t, []int{0, 1, 2, 3, 0}, GetAndFilterGroup([]int{0, 1, 2, 3, 0}))
	assert.Equal(t, []string{"", "a", "", "b", "c"}, GetAndFilterGroup([]string{"", "a", "", "b", "c"}))
	assert.Len(t, GetAndFilterGroup([]dummy1{{}, {}}), 2)

	// filter interface{}
	assert.Equal(t, []interface{}{"abc"}, GetAndFilterGroup([]interface{}{nil, "abc", nil}))

	// filter pointers
	d1 := &dummy1{}
	pointerList := []*dummy1{nil, d1, nil}

	require.Len(t, GetAndFilterGroup(pointerList), 1)
	assert.Equal(t, d1, GetAndFilterGroup(pointerList)[0])

	// filter map
	mapList := []map[string]int{{"1": 1}, nil}

	require.Len(t, GetAndFilterGroup(mapList), 1)
	assert.NotNil(t, GetAndFilterGroup(mapList)[0])

	// filter slice
	arrayList := [][]int{{1, 2, 3, 4}, nil}

	require.Len(t, GetAndFilterGroup(arrayList), 1)
	assert.NotNil(t, GetAndFilterGroup(arrayList)[0])

	// filter chan
	chanList := []chan int{make(chan int), nil}

	require.Len(t, GetAndFilterGroup(chanList), 1)
	assert.NotNil(t, GetAndFilterGroup(chanList)[0])

	// filter func
	funcList := []dummyFunc{func() {}, nil}

	require.Len(t, GetAndFilterGroup(funcList), 1)
	assert.NotNil(t, GetAndFilterGroup(funcList)[0])

	// filter interface
	d2 := &dummy2{}
	d3 := dummy3{}
	interfaceList := []dummyInterface{nil, d1, d2, d3, nil}
	filtered := GetAndFilterGroup(interfaceList)

	require.Len(t, filtered, 3)
	assert.Equal(t, d1, filtered[0])
	assert.Equal(t, d2, filtered[1])
	assert.Equal(t, d3, filtered[2])
}
