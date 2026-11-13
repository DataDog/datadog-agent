// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestAnomalyDetectionLimiterPerWorkload(t *testing.T) {
	limiter, err := NewAnomalyDetectionLimiter(10, 1, time.Minute)
	require.NoError(t, err)

	evWorkloadA := NewCustomEvent(model.ExecEventType, nil).WithWorkloadID("workload-a")
	evWorkloadB := NewCustomEvent(model.ExecEventType, nil).WithWorkloadID("workload-b")

	assert.Equal(t, "workload-a", evWorkloadA.GetWorkloadID())
	clonedA := evWorkloadA.Clone()
	assert.Equal(t, "workload-a", clonedA.GetWorkloadID())

	assert.True(t, limiter.Allow(evWorkloadA))
	assert.False(t, limiter.Allow(evWorkloadA))

	assert.True(t, limiter.Allow(evWorkloadB))
	assert.False(t, limiter.Allow(evWorkloadB))
}
