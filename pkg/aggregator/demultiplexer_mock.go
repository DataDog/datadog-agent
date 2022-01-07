// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics"
)

// TestAgentDemultiplexer is an implementation of the Demultiplexer which is sending
// the time samples into a fake sampler, you can then use WaitForSamples() to retrieve
// the samples that the TimeSamplers should have received.
type TestAgentDemultiplexer struct {
	*AgentDemultiplexer
	receivedSamples []metrics.MetricSample
	sync.Mutex
}

// AddTimeSamples implements a noop timesampler, appending the sample in an internal slice.
func (a *TestAgentDemultiplexer) AddTimeSamples(shard uint32, samples []metrics.MetricSample) {
	a.Lock()
	a.receivedSamples = append(a.receivedSamples, samples...)
	a.Unlock()
}

func (a *TestAgentDemultiplexer) samples() []metrics.MetricSample {
	a.Lock()
	c := make([]metrics.MetricSample, len(a.receivedSamples))
	for i, s := range a.receivedSamples {
		c[i] = s
	}
	a.Unlock()
	return c
}

// WaitForSamples returns the samples received by the demultiplexer.
func (a *TestAgentDemultiplexer) WaitForSamples(timeout time.Duration) []metrics.MetricSample {
	ticker := time.NewTicker(10 * time.Millisecond)
	timeoutOn := time.Now().Add(timeout)
	for {
		select {
		case <-ticker.C:
			s := a.samples()

			// this case could always take priority on the timeout case, we have to make sure
			// we've not timeout
			if time.Now().After(timeoutOn) {
				return s
			}

			if len(s) > 0 {
				return s
			}
		case <-time.After(timeout):
			return nil
		}
	}
}

// Reset resets the internal samples slice.
func (a *TestAgentDemultiplexer) Reset() {
	a.Lock()
	a.receivedSamples = a.receivedSamples[0:0]
	a.Unlock()
}

// InitTestAgentDemultiplexerWithFlushInterval inits a TestAgentDemultiplexer with the given flush interval.
func InitTestAgentDemultiplexerWithFlushInterval(flushInterval time.Duration) *TestAgentDemultiplexer {
	opts := DefaultDemultiplexerOptions(nil)
	opts.FlushInterval = flushInterval
	opts.DontStartForwarders = true
	demux := InitAndStartAgentDemultiplexer(opts, "hostname")
	testAgent := TestAgentDemultiplexer{
		AgentDemultiplexer: demux,
	}
	return &testAgent
}

// InitTestAgentDemultiplexer inits a TestAgentDemultiplexer with a long flush interval.
func InitTestAgentDemultiplexer() *TestAgentDemultiplexer {
	return InitTestAgentDemultiplexerWithFlushInterval(time.Hour) // long flush interval for unit tests
}
