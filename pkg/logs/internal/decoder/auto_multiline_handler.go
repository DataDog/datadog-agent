// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"time"

	automultilinedetection "github.com/DataDog/datadog-agent/pkg/logs/internal/decoder/auto_multiline_detection"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// AutoMultilineHandler aggreagates multiline logs.
type AutoMultilineHandler struct {
	labler     *automultilinedetection.Labeler
	aggregator *automultilinedetection.Aggregator
}

// NewAutoMultilineHandler creates a new auto multiline handler.
func NewAutoMultilineHandler(outputFn func(m *message.Message), maxContentSize int, flushTimeout time.Duration) *AutoMultilineHandler {

	// Order is important
	heuristics := []automultilinedetection.Heuristic{
		automultilinedetection.NewJSONDetector(),
	}

	return &AutoMultilineHandler{
		labler:     automultilinedetection.NewLabler(heuristics),
		aggregator: automultilinedetection.NewAggregator(outputFn, maxContentSize, flushTimeout),
	}
}

func (a *AutoMultilineHandler) process(msg *message.Message) {
	label := a.labler.Label(msg.GetContent())
	a.aggregator.Aggregate(msg, label)
}

func (a *AutoMultilineHandler) flushChan() <-chan time.Time {
	return a.aggregator.FlushChan()
}

func (a *AutoMultilineHandler) flush() {
	a.aggregator.Flush()
}
