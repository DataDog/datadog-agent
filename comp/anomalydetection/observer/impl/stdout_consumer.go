// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"time"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
)

// StdoutAnomalyEventConsumer is a test/debug AnomalyEventConsumer that prints
// every ScoredAnomalyEvent to stdout. It is intended for use in the testbench
// and unit tests — not for production use.
type StdoutAnomalyEventConsumer struct {
	// Prefix is an optional string prepended to each printed line.
	Prefix string
}

// NewStdoutAnomalyEventConsumer creates a new stdout consumer.
func NewStdoutAnomalyEventConsumer(prefix string) *StdoutAnomalyEventConsumer {
	return &StdoutAnomalyEventConsumer{Prefix: prefix}
}

// Name satisfies AnomalyEventConsumer.
func (c *StdoutAnomalyEventConsumer) Name() string { return "stdout" }

// ProcessAnomalyEvent prints the event to stdout. It is synchronous and fast
// (no network I/O), so it satisfies the non-blocking consumer contract.
func (c *StdoutAnomalyEventConsumer) ProcessAnomalyEvent(evt observerdef.ScoredAnomalyEvent) {
	ts := time.Unix(evt.Anomaly.Timestamp, 0).UTC().Format("15:04:05")
	sc := evt.Score

	prefix := c.Prefix
	if prefix != "" {
		prefix += " "
	}

	severitySymbol := "·"
	switch sc.Severity {
	case observerdef.AnomalyEventSeverityHigh:
		severitySymbol = "▲"
	case observerdef.AnomalyEventSeverityMedium:
		severitySymbol = "●"
	}

	trendSymbol := "→"
	switch sc.Trend {
	case observerdef.AnomalyEventTrendIncreased:
		trendSymbol = "↑"
	case observerdef.AnomalyEventTrendDecreased:
		trendSymbol = "↓"
	}

	changeTag := ""
	if sc.SeverityChanged {
		changeTag = fmt.Sprintf(" [%s→%s]", sc.PreviousSeverity, sc.Severity)
	}

	fmt.Printf(
		"%s[anomaly-event] %s %s%s%s id=%s scope=%s instant=%.3f ewma=%.3f (prev=%.3f) detector=%s source=%s\n",
		prefix,
		ts,
		severitySymbol,
		trendSymbol,
		changeTag,
		evt.ID,
		evt.Scope,
		sc.Instant,
		sc.EWMA,
		sc.PreviousEWMA,
		evt.Anomaly.DetectorName,
		evt.Anomaly.Source.String(),
	)
}
