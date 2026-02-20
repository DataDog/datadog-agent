// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package preprocessor

import "github.com/DataDog/datadog-agent/pkg/logs/message"

// autoMultilineCombiner implements Combiner using a Labeler and Aggregator.
type autoMultilineCombiner struct {
	labeler    *Labeler
	aggregator Aggregator
	collected  []*message.Message
}

// NewAutoMultilineCombiner creates a new autoMultilineCombiner.
//
// The aggregatorFactory is called with a collect outputFn to build the underlying
// Aggregator. This factory pattern is needed because the Aggregator's outputFn must
// append to the combiner's collected slice, but the combiner struct doesn't exist
// until this function runs. The closure captures the pointer once and is safe to call
// from within Process and Flush.
func NewAutoMultilineCombiner(labeler *Labeler, aggregatorFactory func(outputFn func(*message.Message)) Aggregator) Combiner {
	c := &autoMultilineCombiner{labeler: labeler}
	c.aggregator = aggregatorFactory(func(msg *message.Message) {
		c.collected = append(c.collected, msg)
	})
	return c
}

// Process labels the message and passes it to the aggregator.
// Returns any messages the aggregator emitted in response.
func (c *autoMultilineCombiner) Process(msg *message.Message) []*message.Message {
	c.collected = c.collected[:0]
	label := c.labeler.Label(msg)
	c.aggregator.Process(msg, label)
	return c.collected
}

// Flush flushes the aggregator and returns any pending messages.
func (c *autoMultilineCombiner) Flush() []*message.Message {
	c.collected = c.collected[:0]
	c.aggregator.Flush()
	return c.collected
}

// IsEmpty returns true if the aggregator has no buffered data.
func (c *autoMultilineCombiner) IsEmpty() bool {
	return c.aggregator.IsEmpty()
}
