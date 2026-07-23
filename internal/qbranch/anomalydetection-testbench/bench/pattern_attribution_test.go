// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package bench

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAttributionTokensMatchUsesShortestSequenceLikeAdaptiveSampler(t *testing.T) {
	assert.True(t, attributionTokensMatch(
		[]string{"a", "b"},
		[]string{"a", "b", "c", "d"},
		0.9,
	))
	assert.False(t, attributionTokensMatch(
		[]string{"a", "x", "c"},
		[]string{"a", "b", "c"},
		0.9,
	))
}

func TestAttributionConstructs(t *testing.T) {
	features := attributionConstructs(`{"event_id":"c05d056c-1c1f-457f-bfd2-f381f7f84e0d","trace_id":"6884385773749550703"}`)
	assert.ElementsMatch(t, []string{"json", "uuid", "long_hex", "long_integer"}, features)
}

func TestObserverFeedbackIdentification(t *testing.T) {
	assert.True(t, isObserverFeedbackLog("[observer] sending change event"))
	assert.True(t, isObserverFeedbackLog("Correlated behavior change detected: 2 anomalies"))
	assert.False(t, isObserverFeedbackLog("application failed to connect"))
}
