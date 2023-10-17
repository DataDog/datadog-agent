// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package aggregator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/check/stats"
	"github.com/fatih/color"
	"github.com/olekukonko/tablewriter"
)

// AgentDemultiplexerPrinter is used to output series, sketches, service checks
// and events.
// Today, this is only used by the `agent check` command.
type AgentDemultiplexerPrinter struct {
	*AgentDemultiplexer
}

type eventPlatformDebugEvent struct {
	RawEvent          string `json:",omitempty"`
	EventType         string
	UnmarshalledEvent map[string]interface{} `json:",omitempty"`
}

// PrintMetrics prints metrics aggregator in the Demultiplexer's check samplers (series and sketches),
// service checks buffer, events buffers.
func (p AgentDemultiplexerPrinter) PrintMetrics(checkFileOutput *bytes.Buffer, formatTable bool) {
	series, sketches := p.aggregator.GetSeriesAndSketches(time.Now())
	if len(series) != 0 {
		fmt.Fprintf(color.Output, "=== %s ===\n", color.BlueString("Series"))

		if formatTable {
			headers, data := series.MarshalStrings()
			var buffer bytes.Buffer

			// plain table with no borders
			table := tablewriter.NewWriter(&buffer)
			table.SetHeader(headers)
			table.SetAutoWrapText(false)
			table.SetAutoFormatHeaders(true)
			table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
			table.SetAlignment(tablewriter.ALIGN_LEFT)
			table.SetCenterSeparator("")
			table.SetColumnSeparator("")
			table.SetRowSeparator("")
			table.SetHeaderLine(false)
			table.SetBorder(false)
			table.SetTablePadding("\t")

			table.AppendBulk(data)
			table.Render()
			fmt.Println(buffer.String())
			checkFileOutput.WriteString(buffer.String() + "\n")
		} else {
			j, _ := json.MarshalIndent(series, "", "  ")
			fmt.Println(string(j))
			checkFileOutput.WriteString(string(j) + "\n")
		}
	}
	if len(sketches) != 0 {
		fmt.Fprintf(color.Output, "=== %s ===\n", color.BlueString("Sketches"))
		j, _ := json.MarshalIndent(sketches, "", "  ")
		fmt.Println(string(j))
		checkFileOutput.WriteString(string(j) + "\n")
	}

	serviceChecks := p.aggregator.GetServiceChecks()
	if len(serviceChecks) != 0 {
		fmt.Fprintf(color.Output, "=== %s ===\n", color.BlueString("Service Checks"))

		if formatTable {
			headers, data := serviceChecks.MarshalStrings()
			var buffer bytes.Buffer

			// plain table with no borders
			table := tablewriter.NewWriter(&buffer)
			table.SetHeader(headers)
			table.SetAutoWrapText(false)
			table.SetAutoFormatHeaders(true)
			table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
			table.SetAlignment(tablewriter.ALIGN_LEFT)
			table.SetCenterSeparator("")
			table.SetColumnSeparator("")
			table.SetRowSeparator("")
			table.SetHeaderLine(false)
			table.SetBorder(false)
			table.SetTablePadding("\t")

			table.AppendBulk(data)
			table.Render()
			fmt.Println(buffer.String())
			checkFileOutput.WriteString(buffer.String() + "\n")
		} else {
			j, _ := json.MarshalIndent(serviceChecks, "", "  ")
			fmt.Println(string(j))
			checkFileOutput.WriteString(string(j) + "\n")
		}
	}

	events := p.aggregator.GetEvents()
	if len(events) != 0 {
		fmt.Fprintf(color.Output, "=== %s ===\n", color.BlueString("Events"))
		checkFileOutput.WriteString("=== Events ===\n")
		j, _ := json.MarshalIndent(events, "", "  ")
		fmt.Println(string(j))
		checkFileOutput.WriteString(string(j) + "\n")
	}

	for k, v := range p.toDebugEpEvents() {
		if len(v) > 0 {
			if translated, ok := stats.EventPlatformNameTranslations[k]; ok {
				k = translated
			}
			fmt.Fprintf(color.Output, "=== %s ===\n", color.BlueString(k))
			checkFileOutput.WriteString(fmt.Sprintf("=== %s ===\n", k))
			j, _ := json.MarshalIndent(v, "", "  ")
			fmt.Println(string(j))
			checkFileOutput.WriteString(string(j) + "\n")
		}
	}
}

// toDebugEpEvents transforms the raw event platform messages to eventPlatformDebugEvents which are better for json formatting
func (p AgentDemultiplexerPrinter) toDebugEpEvents() map[string][]eventPlatformDebugEvent {
	events := p.aggregator.GetEventPlatformEvents()
	result := make(map[string][]eventPlatformDebugEvent)
	for eventType, messages := range events {
		var events []eventPlatformDebugEvent
		for _, m := range messages {
			e := eventPlatformDebugEvent{EventType: eventType, RawEvent: string(m.GetContent())}
			err := json.Unmarshal([]byte(e.RawEvent), &e.UnmarshalledEvent)
			if err == nil {
				e.RawEvent = ""
			}
			events = append(events, e)
		}
		result[eventType] = events
	}
	return result
}

// GetMetricsDataForPrint returns metrics data for series and sketches for printing purpose.
func (p AgentDemultiplexerPrinter) GetMetricsDataForPrint() map[string]interface{} {
	aggData := make(map[string]interface{})

	agg := p.Aggregator()

	series, sketches := agg.GetSeriesAndSketches(time.Now())
	if len(series) != 0 {
		metrics := make([]interface{}, len(series))
		// Workaround to get the sequence of metrics as plain interface{}
		for i, serie := range series {
			serie.PopulateDeviceField()
			serie.PopulateResources()
			sj, _ := json.Marshal(serie)
			json.Unmarshal(sj, &metrics[i]) //nolint:errcheck
		}

		aggData["metrics"] = metrics
	}
	if len(sketches) != 0 {
		aggData["sketches"] = sketches
	}

	serviceChecks := agg.GetServiceChecks()
	if len(serviceChecks) != 0 {
		aggData["service_checks"] = serviceChecks
	}

	events := agg.GetEvents()
	if len(events) != 0 {
		aggData["events"] = events
	}

	for k, v := range p.toDebugEpEvents() {
		aggData[k] = v
	}

	return aggData
}
