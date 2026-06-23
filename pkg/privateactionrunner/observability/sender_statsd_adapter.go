// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package observability

import (
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
)

// senderStatsdAdapter implements the subset of statsd.ClientInterface that the
// Private Action Runner uses (Incr, Timing) by submitting metrics in-process to
// an aggregator Sender, instead of sending DogStatsD packets over a socket/UDP.
//
// This lets PAR emit metrics through the Cluster Agent's existing demultiplexer
// (which already aggregates and forwards to the backend) without a DogStatsD
// server or a node-agent hostPath socket — which is unavailable on a node where
// the DCA runs but no node Agent does (e.g. a multi-node cluster).
//
// The embedded NoOpClient satisfies the full statsd.ClientInterface; only the
// two methods PAR actually calls are overridden.
type senderStatsdAdapter struct {
	statsd.ClientInterface
	sender sender.Sender
}

var _ statsd.ClientInterface = (*senderStatsdAdapter)(nil)

// NewSenderStatsdAdapter returns a statsd.ClientInterface backed by an in-process
// aggregator Sender.
func NewSenderStatsdAdapter(s sender.Sender) statsd.ClientInterface {
	return &senderStatsdAdapter{
		ClientInterface: &statsd.NoOpClient{},
		sender:          s,
	}
}

// Incr submits a counter increment. The sampling rate is ignored: in-process
// submission is not sampled, and PAR always passes 1.0. An empty hostname lets
// the aggregator fill it in.
func (a *senderStatsdAdapter) Incr(name string, tags []string, _ float64) error {
	a.sender.Count(name, 1, "", tags)
	a.sender.Commit()
	return nil
}

// Timing submits a duration as a histogram of milliseconds, matching DogStatsD's
// Timing semantics.
func (a *senderStatsdAdapter) Timing(name string, value time.Duration, tags []string, _ float64) error {
	a.sender.Histogram(name, float64(value)/float64(time.Millisecond), "", tags)
	a.sender.Commit()
	return nil
}
