// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package ksm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReset(t *testing.T) {
	tc := newTelemetryCache()
	tc.totalCount = 1337
	tc.unknownMetricsCount = 1337
	tc.metricsCountByResource = map[string]int{"foo": 1337}
	tc.reset()
	assert.Equal(t, 0, tc.totalCount)
	assert.Equal(t, 0, tc.unknownMetricsCount)
	assert.Len(t, tc.metricsCountByResource, 0)
}

func TestTotal(t *testing.T) {
	tc := newTelemetryCache()
	tc.incTotal(1337)
	tc.incTotal(1337)
	assert.Equal(t, 1337+1337, tc.getTotal())
}

func TestUnknown(t *testing.T) {
	tc := newTelemetryCache()
	tc.incUnknown()
	tc.incUnknown()
	assert.Equal(t, 2, tc.getUnknown())
}

func TestCountByResource(t *testing.T) {
	tc := newTelemetryCache()
	tc.incResource("foo", 1)
	tc.incResource("bar", 1)
	tc.incResource("foo", 1)
	assert.Equal(t, 2, tc.getResourcesCount()["foo"])
	assert.Equal(t, 1, tc.getResourcesCount()["bar"])
	assert.Len(t, tc.metricsCountByResource, 2)
}
