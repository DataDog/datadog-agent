// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

import (
	"bytes"
	"slices"

	"github.com/davecgh/go-spew/spew"
	"github.com/stretchr/testify/assert"
	"testing"
)

var spewConfig = spew.ConfigState{
	Indent:                  " ",
	DisablePointerAddresses: true,
	DisableCapacities:       true,
	SortKeys:                true,
	DisableMethods:          true,
	MaxDepth:                10,
}

// diffListsFunc compares two lists and returns the elements that are in the first list but not in the second, and vice versa,
// using a custom comparison function.
func diffListsFunc[T any](a []T, b []T, comp func(a, b T) bool) ([]T, []T) {
	extraA := []T{}
	extraB := []T{}
	inBoth := make([]bool, len(b))
	for _, aVal := range a {
		found := false
		for j, bVal := range b {
			if inBoth[j] {
				continue
			}
			if comp(aVal, bVal) {
				inBoth[j] = true
				found = true
				break
			}
		}
		if !found {
			extraA = append(extraA, aVal)
		}
	}
	for j, bVal := range b {
		if inBoth[j] {
			continue
		}
		extraB = append(extraB, bVal)
	}
	return extraA, extraB
}

// formatListDiff returns a string representation of the differences between two lists
func formatListDiff[T any](expected, actual, extraExpected, extraActual []T) string {
	var msg bytes.Buffer

	msg.WriteString("list elements differ")
	if len(extraExpected) > 0 {
		msg.WriteString("\n\nextra elements in expected:\n")
		spewConfig.Fdump(&msg, extraExpected)
	}
	if len(extraActual) > 0 {
		msg.WriteString("\n\nextra elements in actual:\n")
		spewConfig.Fdump(&msg, extraActual)
	}
	msg.WriteString("\n\nexpected:\n")
	spewConfig.Fdump(&msg, expected)
	msg.WriteString("\n\nactual:\n")
	spewConfig.Fdump(&msg, actual)

	return msg.String()
}

func formatElementNotFound[T any](elem T, list []T) string {
	var msg bytes.Buffer

	msg.WriteString("element not found")
	msg.WriteString("\n\nexpected element:\n")
	spewConfig.Fdump(&msg, elem)
	msg.WriteString("\n\nactual list:\n")
	spewConfig.Fdump(&msg, list)

	return msg.String()
}

// AssertElementsMatchFunc is similar to the assert.ElementsMatch function, but it allows for a custom comparison function
func AssertElementsMatchFunc[T any](t *testing.T, expected, actual []T, comp func(a, b T) bool, msgAndArgs ...any) bool {
	t.Helper()

	extraA, extraB := diffListsFunc(expected, actual, comp)
	if len(extraA) == 0 && len(extraB) == 0 {
		return true
	}

	return assert.Fail(t, formatListDiff(expected, actual, extraA, extraB), msgAndArgs...)
}

// AssertContainsFunc is similar to the assert.Contains function, but it allows for a custom comparison function.
// It is also similar to slices.ContainsFunc, which it uses internally, but also provides a helpful error message
// in the style of the testify assert functions.
func AssertContainsFunc[T any](t *testing.T, list []T, elem T, comp func(a, b T) bool, msgAndArgs ...any) bool {
	t.Helper()
	if slices.ContainsFunc(list, func(e T) bool {
		return comp(e, elem)
	}) {
		return true
	}
	return assert.Fail(t, formatElementNotFound(elem, list), msgAndArgs...)
}

type equalable[T any] interface {
	Equal(other T) bool
}

// AssertEqualableElementsMatch is a helper for AssertElementsMatchFunc that works with types that implement the Equal method
func AssertEqualableElementsMatch[T equalable[T]](t *testing.T, expected, actual []T, msgAndArgs ...any) bool {
	t.Helper()
	return AssertElementsMatchFunc(t, expected, actual, func(a, b T) bool {
		return a.Equal(b)
	}, msgAndArgs...)
}

// AssertContainsEqualable is a helper for AssertContainsFunc that works with types that implement the Equal method
func AssertContainsEqualable[T equalable[T]](t *testing.T, list []T, elem T, msgAndArgs ...any) bool {
	t.Helper()
	return AssertContainsFunc(t, list, elem, func(a, b T) bool {
		return a.Equal(b)
	}, msgAndArgs...)
}
