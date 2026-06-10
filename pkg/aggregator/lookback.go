// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	"github.com/DataDog/datadog-agent/pkg/serializer"
)

// LookbackDumper dumps retained metric lookback samples and returns the number
// of series sent.
type LookbackDumper func() (int, error)

// LookbackTrigger observes DogStatsD samples and optionally triggers a lookback
// dump. It is injected into the demultiplexer by binaries that support trigger
// evaluation so binaries that only reuse the aggregator do not link the
// concrete trigger implementation.
type LookbackTrigger interface {
	Observe(name string, value float64) bool
}

// LookbackTriggerFactory builds a trigger after the demultiplexer has been
// constructed, so the trigger callback can use the provided dump function
// without forcing pkg/aggregator to link concrete lookback packages.
type LookbackTriggerFactory func(LookbackDumper) LookbackTrigger

// LookbackRetention owns metric lookback retention outside the normal metric
// aggregation path. It is injected into the demultiplexer by binaries that
// support lookback so binaries that only reuse the aggregator, such as
// standalone DogStatsD, do not link the concrete lookback implementation.
type LookbackRetention interface {
	// NewSenderManager returns a shadow-check sender manager backed by the
	// shared retention buffer. The context is scoped to the shadow check using
	// the returned manager so in-flight writes can observe cancellation.
	NewSenderManager(context.Context) sender.SenderManager

	// Dump sends retained samples through the provided serializer and returns
	// the number of series sent.
	Dump(serializer.MetricSerializer) (int, error)
}
