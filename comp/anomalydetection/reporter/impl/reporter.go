// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package reporterimpl provides the live reporter implementations:
// a stdout reporter (always active) and an optional Datadog event reporter
// (active when anomaly_detection.reporting.enabled=true).
package reporterimpl

import (
	"fmt"
	"strings"
	"time"

	reporterdef "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
)

// Requires defines the dependencies for the live reporter component.
type Requires struct {
	Config        config.Component
	Log           log.Component
	EventPlatform eventplatform.Component
	Hostname      hostname.Component
}

// Provides defines the output of the live reporter component.
// Reporters are provided via the anomalydetection_reporters Fx group so the
// observer can subscribe multiple reporters independently.
type Provides struct {
	Reporters []reporterdef.Reporter `group:"anomalydetection_reporters,flatten"`
}

// NewComponent creates the live reporter component. It always provides a
// stdoutReporter and, when anomaly_detection.reporting.enabled=true and the
// event-platform forwarder is available, also provides an EventReporter that
// posts Datadog change events through the event-management intake pipeline.
func NewComponent(req Requires) (Provides, error) {
	reporters := []reporterdef.Reporter{&stdoutReporter{}}

	if req.Config.GetBool("anomaly_detection.reporting.enabled") {
		forwarder, ok := req.EventPlatform.Get()
		if !ok {
			req.Log.Warnf("[reporter] event_reporter disabled: event-platform forwarder is not running")
		} else {
			sender, err := newEventSender(forwarder, req.Log, nil, req.Hostname)
			if err != nil {
				req.Log.Warnf("[reporter] event_reporter disabled: %v", err)
			} else {
				reporters = append(reporters, &EventReporter{sender: sender, logger: req.Log})
			}
		}
	}

	return Provides{Reporters: reporters}, nil
}

type stdoutReporter struct{}

func (r *stdoutReporter) Name() string { return "stdout_reporter" }

func (r *stdoutReporter) Report(output reporterdef.ReportOutput) {
	if len(output.ActiveCorrelations) == 0 {
		return
	}
	for _, ac := range output.ActiveCorrelations {
		fmt.Printf("[observer] report: pattern=%s — %s (%d series)\n",
			ac.Pattern, ac.Title, len(ac.Members))
		for _, a := range ac.Anomalies {
			ts := time.Unix(a.Timestamp, 0).UTC().Format(time.RFC3339)
			fmt.Printf("  - %s [%s] at %s\n", a.Source.DisplayName(), a.DetectorName, ts)
		}
	}
	if len(output.NewAnomalies) > 0 {
		names := make([]string, 0, len(output.NewAnomalies))
		for _, a := range output.NewAnomalies {
			names = append(names, a.DetectorName+":"+a.Source.Name)
		}
		fmt.Printf("[observer] new anomalies: %s\n", strings.Join(names, ", "))
	}
}
