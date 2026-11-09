// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build oracle

package oracle

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCustomQueryWithoutCollectionIntervalAlwaysExecutes(t *testing.T) {
	var lastExecutionTime time.Time
	now := time.Unix(100, 0)

	assert.True(t, shouldExecuteCustomQuery(&lastExecutionTime, nil, now))
	assert.True(t, shouldExecuteCustomQuery(&lastExecutionTime, nil, now))
	assert.True(t, lastExecutionTime.IsZero())
}

func TestCustomQueryCollectionInterval(t *testing.T) {
	collectionInterval := int64(30)
	var lastExecutionTime time.Time
	firstRun := time.Unix(100, 0)

	assert.True(t, shouldExecuteCustomQuery(&lastExecutionTime, &collectionInterval, firstRun))
	assert.Equal(t, firstRun, lastExecutionTime)
	assert.False(t, shouldExecuteCustomQuery(&lastExecutionTime, &collectionInterval, firstRun.Add(29*time.Second)))
	assert.True(t, shouldExecuteCustomQuery(&lastExecutionTime, &collectionInterval, firstRun.Add(30*time.Second)))
}

func TestCustomQueryCollectionIntervalDoesNotOverflow(t *testing.T) {
	collectionInterval := int64(1<<63 - 1)
	var lastExecutionTime time.Time
	firstRun := time.Unix(100, 0)

	assert.True(t, shouldExecuteCustomQuery(&lastExecutionTime, &collectionInterval, firstRun))
	assert.False(t, shouldExecuteCustomQuery(&lastExecutionTime, &collectionInterval, firstRun.Add(time.Hour)))
}

func TestCustomQueryCollectionIntervalsAreIndependent(t *testing.T) {
	shortInterval := int64(30)
	longInterval := int64(60)
	var shortLastRun time.Time
	var longLastRun time.Time
	firstRun := time.Unix(100, 0)

	assert.True(t, shouldExecuteCustomQuery(&shortLastRun, &shortInterval, firstRun))
	assert.True(t, shouldExecuteCustomQuery(&longLastRun, &longInterval, firstRun))

	secondRun := firstRun.Add(30 * time.Second)
	assert.True(t, shouldExecuteCustomQuery(&shortLastRun, &shortInterval, secondRun))
	assert.False(t, shouldExecuteCustomQuery(&longLastRun, &longInterval, secondRun))

	thirdRun := firstRun.Add(60 * time.Second)
	assert.True(t, shouldExecuteCustomQuery(&shortLastRun, &shortInterval, thirdRun))
	assert.True(t, shouldExecuteCustomQuery(&longLastRun, &longInterval, thirdRun))
}
