// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestInsert(t *testing.T) {
	sketchMap := make(sketchMap)

	assert.Equal(t, 0, sketchMap.Len())

	mSample1 := metrics.MetricSample{
		Name:       "test.metric.name1",
		Value:      1,
		Mtype:      metrics.DistributionType,
		Tags:       []string{"a", "b"},
		SampleRate: 1,
	}

	sketchMap.insert(1, generateContextKey(&mSample1), 1, 1)
	assert.Equal(t, 1, sketchMap.Len())
	sketchMap.insert(2, generateContextKey(&mSample1), 2, 1)
	assert.Equal(t, 2, sketchMap.Len())
}
