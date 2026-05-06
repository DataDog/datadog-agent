// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package reporterimpl implements the reporter component.
package reporterimpl

import (
	reporter "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
)

// Requires defines the dependencies for the reporter component.
type Requires struct {
	Config config.Component
	Log    log.Component
}

// Provides defines the output of the reporter component.
type Provides struct {
	Comp      reporter.Component
	Reporters []reporter.Reporter `group:"anomalydetection_reporters,flatten"`
}

type reporterImpl struct{}

// NewComponent creates the reporter component and initializes all enabled reporters.
func NewComponent(req Requires) (Provides, error) {
	reporters := []reporter.Reporter{
		&StdoutReporter{},
	}

	if req.Config.GetBool("observer.event_reporter.sending_enabled") {
		// Storage is nil until the engine is extracted as its own component (Step 1 of the split plan).
		// Rate display in event messages falls back to DebugInfo.CurrentValue in the interim.
		sender, err := NewEventSender(req.Config, req.Log, nil)
		if err != nil {
			req.Log.Warnf("[reporter] event_reporter disabled: %v", err)
		} else {
			reporters = append(reporters, &EventReporter{sender: sender, logger: req.Log})
		}
	}

	return Provides{
		Comp:      &reporterImpl{},
		Reporters: reporters,
	}, nil
}
