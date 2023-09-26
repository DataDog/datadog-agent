// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package client holds the client to send data to the Cluster-Agent
package client

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/backoff"
	"github.com/DataDog/datadog-agent/pkg/util/clusteragent"
	"github.com/benbjohnson/clock"
)

func (c *client) startFlushing() {
	periodicFlushTimer := time.NewTicker(c.flushPeriod)
	defer periodicFlushTimer.Stop()
	for {
		select {
		case <-periodicFlushTimer.C:
			c.flush()
		case <-c.ctx.Done():
			return
		}
	}

}

func (c *client) flush() {
	if c.langDetectionCl == nil {
		dcaClient, err := clusteragent.GetClusterAgentClient()
		if err != nil {
			c.logger.Errorf("failed to get dca client %s", err)
			return
		}
		c.langDetectionCl = dcaClient
	}
	clock := clock.New()
	errorCount := 0
	backoffPolicy := backoff.NewExpBackoffPolicy(minBackoffFactor, baseBackoffTime.Seconds(), maxBackoffTime.Seconds(), 0, false)

	c.mutex.Lock()
	batch := c.currentBatch
	c.currentBatch = newBatch()
	c.mutex.Unlock()

	protoMessage := batch.toProto()
	for {
		if errorCount >= maxError {
			c.logger.Errorf("failed to send language metadata after %d errors", errorCount)
			c.mergeBatchesAfterError(batch)
			return
		}
		refreshInterval := backoffPolicy.GetBackoffDuration(errorCount)
		select {
		case <-clock.After(refreshInterval):
			t := time.Now()
			err := c.langDetectionCl.PostLanguageMetadata(c.ctx, protoMessage)
			if err == nil {
				c.telemetry.Latency.Observe(time.Since(t).Seconds())
				c.telemetry.Requests.Inc(statusSuccess)
				return
			}
			c.telemetry.Requests.Inc(statusError)
			errorCount = backoffPolicy.IncError(1)
		case <-c.ctx.Done():
			return
		}
	}
}

func (c *client) mergeBatchesAfterError(b *batch) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.currentBatch.merge(b)
}
