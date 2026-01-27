// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package workload

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestProgressTrackerUpdateFirstCall(t *testing.T) {
	tr := newRolloutProgressTracker()
	assert.False(t, tr.Update("id", "r1", 1, time.Now()))
	assert.Equal(t, int32(1), tr.entries["id"].podCount)
}

func TestProgressTrackerUpdateWithProgress(t *testing.T) {
	now := time.Now()
	tr := newRolloutProgressTracker()
	tr.Update("id", "r1", 1, now)
	assert.False(t, tr.Update("id", "r1", 2, now.Add(61*time.Minute)))
	assert.Equal(t, int32(2), tr.entries["id"].podCount)
}

func TestProgressTrackerUpdateNoProgress(t *testing.T) {
	now := time.Now()
	tr := newRolloutProgressTracker()
	tr.Update("id", "r1", 1, now)
	assert.True(t, tr.Update("id", "r1", 1, now.Add(61*time.Minute)))
}

func TestProgressTrackerUpdateDifferentRecommendation(t *testing.T) {
	now := time.Now()
	tr := newRolloutProgressTracker()
	tr.Update("id", "old", 2, now)
	assert.False(t, tr.Update("id", "new", 0, now.Add(61*time.Minute)))
	assert.Equal(t, "new", tr.entries["id"].recommendationID)
}

func TestProgressTrackerClear(t *testing.T) {
	tr := newRolloutProgressTracker()
	tr.Update("id", "r1", 1, time.Now())
	tr.Clear("id")
	assert.Empty(t, tr.entries)
}
