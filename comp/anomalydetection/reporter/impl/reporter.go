// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package reporterimpl provides the live reporter component implementation.
// It prints correlation reports to stdout.
package reporterimpl

import (
	"fmt"
	"strings"

	reporter "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

// Requires defines the dependencies for the live reporter component.
type Requires struct {
	Lifecycle compdef.Lifecycle
}

// Provides defines the output of the live reporter component.
type Provides struct {
	Comp reporter.Component
}

// NewComponent creates the live reporter component (stdout).
func NewComponent(_ Requires) (Provides, error) {
	return Provides{Comp: &stdoutReporter{}}, nil
}

type stdoutReporter struct{}

func (r *stdoutReporter) Name() string { return "stdout_reporter" }

func (r *stdoutReporter) Report(output reporter.ReportOutput) {
	if len(output.ActiveCorrelations) == 0 {
		return
	}
	for _, ac := range output.ActiveCorrelations {
		members := fmt.Sprintf("%d series", ac.MemberCount)
		fmt.Printf("[observer] correlation: %s — %s (%s)\n",
			ac.Pattern, ac.Title, members)
	}
	if len(output.NewAnomalies) > 0 {
		names := make([]string, 0, len(output.NewAnomalies))
		for _, a := range output.NewAnomalies {
			names = append(names, a.DetectorName+":"+a.SeriesName)
		}
		fmt.Printf("[observer] new anomalies: %s\n", strings.Join(names, ", "))
	}
}
