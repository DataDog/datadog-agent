// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package automultilinedetection contains auto multiline detection and aggregation logic.
package automultilinedetection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

type mockHeuristic struct {
	processFunc func(*messageContext) bool
}

func (m *mockHeuristic) ProcessAndContinue(context *messageContext) bool {
	return m.processFunc(context)
}

func TestLabelerProceedNextHeuristic(t *testing.T) {

	labeler := NewLabeler([]Heuristic{
		&mockHeuristic{
			processFunc: func(context *messageContext) bool {
				context.label = startGroup
				return true
			},
		},
		&mockHeuristic{
			processFunc: func(context *messageContext) bool {
				context.label = noAggregate
				return true
			},
		},
	})

	assert.Equal(t, noAggregate, labeler.Label([]byte("test 123")))
}

func TestLabelerProceedFirstHeuristicWins(t *testing.T) {

	labeler := NewLabeler([]Heuristic{
		&mockHeuristic{
			processFunc: func(context *messageContext) bool {
				context.label = startGroup
				return false
			},
		},
		&mockHeuristic{
			processFunc: func(context *messageContext) bool {
				context.label = noAggregate
				return true
			},
		},
	})

	assert.Equal(t, startGroup, labeler.Label([]byte("test 123")))
}

func TestLabelerDefaultLabel(t *testing.T) {

	labeler := NewLabeler([]Heuristic{
		&mockHeuristic{
			processFunc: func(*messageContext) bool {
				return false
			},
		},
	})

	assert.Equal(t, aggregate, labeler.Label([]byte("test 123")))
}

func TestLabelerPassesAlongMessageContext(t *testing.T) {

	labeler := NewLabeler([]Heuristic{
		&mockHeuristic{
			processFunc: func(context *messageContext) bool {
				assert.Equal(t, "test 123", string(context.rawMessage))
				return false
			},
		},
	})

	labeler.Label([]byte("test 123"))
}
