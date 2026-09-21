// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustermetadata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestReducePeerAnswers tests the reduction of a broadcast lookup.
// Test partitions:
// - presence of Found: present | absent
// - NotReady with no Found: present | absent
// - Absent with no Found/NotReady: present | absent
// - only NotMine: present | empty input (boundary: zero answers)
func TestReducePeerAnswers(t *testing.T) {
	assert.Equal(t,
		LookupAnswer{Kind: AnswerFound, Tags: []string{"kube_namespace:default"}},
		ReducePeerAnswers([]LookupAnswer{
			{Kind: AnswerNotMine},
			{Kind: AnswerFound, Tags: []string{"kube_namespace:default"}},
			{Kind: AnswerAbsent},
		}),
		"first Found wins over NotReady, Absent and NotMine")

	assert.Equal(t,
		LookupAnswer{Kind: AnswerFound},
		ReducePeerAnswers([]LookupAnswer{{Kind: AnswerFound}}),
		"Found with nil tags is a valid answer and returned as is")

	assert.Equal(t,
		LookupAnswer{Kind: AnswerNotReady},
		ReducePeerAnswers([]LookupAnswer{{Kind: AnswerNotMine}, {Kind: AnswerAbsent}, {Kind: AnswerNotReady}}),
		"NotReady wins over Absent: a stale Absent must not surface as absent")

	assert.Equal(t,
		LookupAnswer{Kind: AnswerAbsent},
		ReducePeerAnswers([]LookupAnswer{{Kind: AnswerNotMine}, {Kind: AnswerNotMine}, {Kind: AnswerAbsent}}),
		"authoritative synced Absent wins over NotMine")

	assert.Equal(t,
		LookupAnswer{Kind: AnswerAbsent},
		ReducePeerAnswers([]LookupAnswer{{Kind: AnswerNotMine}, {Kind: AnswerNotMine}}),
		"all NotMine reduces to Absent: every synced replica checked its cache")

	assert.Equal(t,
		LookupAnswer{Kind: AnswerNotReady},
		ReducePeerAnswers(nil),
		"no answers (boundary) reduces to NotReady")
}

// TestReducePeerAnswersOrder tests Found precedence within mixed answers.
// Test partitions:
// - position of Found: first | last (boundary: first and last element)
func TestReducePeerAnswersOrder(t *testing.T) {
	first := []LookupAnswer{{Kind: AnswerFound, Tags: []string{"a"}}, {Kind: AnswerNotReady}, {Kind: AnswerAbsent}}
	last := []LookupAnswer{{Kind: AnswerNotReady}, {Kind: AnswerAbsent}, {Kind: AnswerFound, Tags: []string{"a"}}}

	assert.Equal(t, LookupAnswer{Kind: AnswerFound, Tags: []string{"a"}}, ReducePeerAnswers(first), "Found at the first position")
	assert.Equal(t, LookupAnswer{Kind: AnswerFound, Tags: []string{"a"}}, ReducePeerAnswers(last), "Found at the last position")
}
