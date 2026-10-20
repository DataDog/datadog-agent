// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

type anomalyLimiterTestEvent struct {
	workloadID string
}

func (e anomalyLimiterTestEvent) GetWorkloadID() string {
	return e.workloadID
}

func (anomalyLimiterTestEvent) GetTags() []string {
	return nil
}

func (anomalyLimiterTestEvent) GetType() string {
	return ""
}

func (anomalyLimiterTestEvent) GetActionReports() []model.ActionReport {
	return nil
}

func (anomalyLimiterTestEvent) GetFieldValue(eval.Field) (interface{}, error) {
	return nil, nil
}

func TestAnomalyDetectionLimiterConcurrentAllowance(t *testing.T) {
	const (
		calls   = 128
		allowed = 7
	)

	limiter, err := NewAnomalyDetectionLimiter(1, allowed, time.Hour)
	require.NoError(t, err)

	var ready sync.WaitGroup
	var done sync.WaitGroup
	var allowedCalls atomic.Int64
	start := make(chan struct{})
	event := anomalyLimiterTestEvent{workloadID: "workload"}

	ready.Add(calls)
	done.Add(calls)
	for range calls {
		go func() {
			defer done.Done()
			ready.Done()
			<-start
			if limiter.Allow(event) {
				allowedCalls.Add(1)
			}
		}()
	}

	ready.Wait()
	close(start)
	done.Wait()

	assert.Equal(t, int64(allowed), allowedCalls.Load())
	stats := limiter.SwapStats()
	require.Len(t, stats, 1)
	assert.Equal(t, uint64(allowed), stats[0].Allowed)
	assert.Equal(t, uint64(calls-allowed), stats[0].Dropped)
}
