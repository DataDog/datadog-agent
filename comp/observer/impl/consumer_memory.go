// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"fmt"
	"strings"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// PassthroughProcessor is a simple anomaly processor that converts each anomaly to a report.
// It serves as an example implementation and for testing.
type PassthroughProcessor struct {
	anomalies []observer.AnomalyOutput
}

// Name returns the processor name.
func (p *PassthroughProcessor) Name() string {
	return "passthrough_processor"
}

// Process adds an anomaly to the pending list.
func (p *PassthroughProcessor) Process(anomaly observer.AnomalyOutput) {
	p.anomalies = append(p.anomalies, anomaly)
}

// Flush converts accumulated anomalies to reports and clears the list.
func (p *PassthroughProcessor) Flush() []observer.ReportOutput {
	if len(p.anomalies) == 0 {
		return nil
	}

	reports := make([]observer.ReportOutput, len(p.anomalies))
	for i, a := range p.anomalies {
		reports[i] = observer.ReportOutput{
			Title: a.Title,
			Body:  a.Description,
			Metadata: map[string]string{
				"tags": strings.Join(a.Tags, ","),
			},
		}
	}

	p.anomalies = nil
	return reports
}

// GetPending returns pending anomalies (for testing).
func (p *PassthroughProcessor) GetPending() []observer.AnomalyOutput {
	return p.anomalies
}

// StdoutReporter prints reports to stdout.
type StdoutReporter struct{}

// Name returns the reporter name.
func (r *StdoutReporter) Name() string {
	return "stdout_reporter"
}

// Report prints a report to stdout.
func (r *StdoutReporter) Report(report observer.ReportOutput) {
	fmt.Printf("[observer] %s: %s", report.Title, report.Body)
	if len(report.Metadata) > 0 {
		fmt.Printf(" (")
		first := true
		for k, v := range report.Metadata {
			if !first {
				fmt.Printf(", ")
			}
			fmt.Printf("%s=%s", k, v)
			first = false
		}
		fmt.Printf(")")
	}
	fmt.Println()
}
