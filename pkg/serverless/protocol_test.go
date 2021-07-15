// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"context"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/aws"
	"github.com/stretchr/testify/assert"
)

func TestWaitForDaemonBlocking(t *testing.T) {
	assert := assert.New(t)
	_, cancel := context.WithCancel(context.Background())
	d := StartDaemon(cancel)
	d.ReadyWg.Done()
	defer d.Stop()

	// WaitForDaemon doesn't block if the client library hasn't
	// registered with the extension's /hello route
	d.clientLibReady = false
	d.WaitForDaemon()

	// WaitForDaemon blocks if the client library has registered with the extension's /hello route
	d.clientLibReady = true

	d.StartInvocation()

	complete := false
	go func() {
		<-time.After(100 * time.Millisecond)
		complete = true
		d.FinishInvocation()
	}()
	d.WaitForDaemon()
	assert.Equal(complete, true, "daemon didn't block until FinishInvocation")
}

func TestWaitUntilReady(t *testing.T) {
	assert := assert.New(t)
	_, cancel := context.WithCancel(context.Background())
	d := StartDaemon(cancel)
	d.ReadyWg.Done()
	defer d.Stop()

	ready := d.WaitUntilClientReady(50 * time.Millisecond)
	assert.Equal(ready, false, "client was ready")
}

func TestProcessMessage(t *testing.T) {
	message := aws.LogMessage{
		Type: aws.LogTypePlatformReport,
		Time: time.Now(),
		ObjectRecord: aws.PlatformObjectRecord{
			Metrics: aws.ReportLogMetrics{
				DurationMs:       1000.0,
				BilledDurationMs: 800.0,
				MemorySizeMB:     1024.0,
				MaxMemoryUsedMB:  256.0,
				InitDurationMs:   100.0,
			},
		},
	}
	arn := "arn:aws:lambda:us-east-1:123456789012:function:test-function"
	lastRequestID := "8286a188-ba32-4475-8077-530cd35c09a9"
	functionName := "test-function"
	metricTags := []string{"functionname:test-function"}

	metricsChan := make(chan []metrics.MetricSample, 1)
	computeEnhancedMetrics := true
	go processMessage(message, arn, lastRequestID, functionName, computeEnhancedMetrics, metricTags, metricsChan)

	select {
	case received := <-metricsChan:
		assert.Equal(t, len(received), 6)
	case <-time.After(time.Second):
		assert.Fail(t, "We should have received metrics")
	}

	metricsChan = make(chan []metrics.MetricSample, 1)
	computeEnhancedMetrics = false
	go processMessage(message, arn, lastRequestID, functionName, computeEnhancedMetrics, metricTags, metricsChan)

	select {
	case <-metricsChan:
		assert.Fail(t, "We should not have received metrics")
	case <-time.After(time.Second):
		//nothing to do here
	}
}
