// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
)

// AgentDemultiplexerPrinter is used to output series, sketches, service checks
// and events.
// Today, this is only used by the `agent check` command.
type AgentDemultiplexerPrinter struct {
	DemultiplexerWithAggregator
}

type eventPlatformDebugEvent struct {
	RawEvent          string `json:",omitempty"`
	EventType         string
	UnmarshalledEvent map[string]interface{} `json:",omitempty"`
}

// PrintMetrics prints metrics aggregator in the Demultiplexer's check samplers (series and sketches),
// service checks buffer, events buffers.
func (p AgentDemultiplexerPrinter) PrintMetrics(checkFileOutput *bytes.Buffer, formatTable bool) {
	panic("not called")
}

// toDebugEpEvents transforms the raw event platform messages to eventPlatformDebugEvents which are better for json formatting
func (p AgentDemultiplexerPrinter) toDebugEpEvents() map[string][]eventPlatformDebugEvent {
	panic("not called")
}

// GetMetricsDataForPrint returns metrics data for series and sketches for printing purpose.
func (p AgentDemultiplexerPrinter) GetMetricsDataForPrint() map[string]interface{} {
	panic("not called")
}
