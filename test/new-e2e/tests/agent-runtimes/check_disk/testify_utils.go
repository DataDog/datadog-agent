// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//Copyright (c) 2012-2020 Mat Ryer, Tyler Bunnell and contributors.

package checkdisk

import (
	"bytes"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/require"
)

// diffLists is a copy of the diffLists function from the testify library
// https://github.com/stretchr/testify/blob/v1.10.0/assert/assertions.go#L1118-L1156
// It is modified to use a custom compare function instead of the default one
// It is also modified to be typed and to modernize the code for simplicity
//
// diffLists diffs two arrays/slices and returns slices of elements that are only in A and only in B.
// If some element is present multiple times, each instance is counted separately (e.g. if something is 2x in A and
// 5x in B, it will be 0x in extraA and 3x in extraB). The order of items in both lists is ignored.
func diffLists[T any](listA []T, listB []T, compareFunc func(a, b T) bool) (extraA, extraB []T) {
	// Mark indexes in listB that we already used
	visited := make([]bool, len(listB))
	for _, itemA := range listA {
		found := false
		for j, itemB := range listB {
			if visited[j] {
				continue
			}
			if compareFunc(itemB, itemA) {
				visited[j] = true
				found = true
				break
			}
		}
		if !found {
			extraA = append(extraA, itemA)
		}
	}

	for j, itemB := range listB {
		if visited[j] {
			continue
		}
		extraB = append(extraB, itemB)
	}

	return
}

func elementsMatch[T any](t require.TestingT, listA, listB []T, compareFunc func(a, b T) bool, msgAndArgs ...interface{}) {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}

	extraA, extraB := diffLists(listA, listB, compareFunc)

	if len(extraA) == 0 && len(extraB) == 0 {
		return
	}

	require.Fail(t, formatListDiff(listA, listB, extraA, extraB), msgAndArgs...)
}

var spewConfig = spew.ConfigState{
	Indent:                  " ",
	DisablePointerAddresses: true,
	DisableCapacities:       true,
	SortKeys:                true,
	DisableMethods:          true,
	MaxDepth:                10,
}

func formatListDiff[T any](listA, listB []T, extraA, extraB []T) string {
	var msg bytes.Buffer

	msg.WriteString("elements differ")
	if len(extraA) > 0 {
		msg.WriteString("\n\nextra elements in list A:\n")
		msg.WriteString(spewConfig.Sdump(extraA))
	}
	if len(extraB) > 0 {
		msg.WriteString("\n\nextra elements in list B:\n")
		msg.WriteString(spewConfig.Sdump(extraB))
	}
	msg.WriteString("\n\nlistA:\n")
	msg.WriteString(spewConfig.Sdump(listA))
	msg.WriteString("\n\nlistB:\n")
	msg.WriteString(spewConfig.Sdump(listB))

	return msg.String()
}
