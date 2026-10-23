// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package phonehomeimpl

import (
	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
	phonehome "github.com/DataDog/datadog-agent/comp/privateactionrunner/phonehome/def"
)

// The controller calls this serially; collection uses the telemetry registry's synchronization.
func newTelemetryReporter(tel telemetry.Component) func(phonehome.State) {
	state := tel.NewGauge("par_phone_home", "state", []string{"state", "reason"},
		"Current PAR startup-gate state, not enrollment or runtime health.")
	transitions := tel.NewCounter("par_phone_home", "transitions", []string{"state", "reason"},
		"Entries into a PAR startup-gate state/reason, including the initial observation.")
	var previous phonehome.State
	return func(s phonehome.State) {
		if s.State == previous.State && s.Reason == previous.Reason {
			return
		}
		if previous.State != "" {
			state.Set(0, previous.State, previous.Reason)
		}
		state.Set(1, s.State, s.Reason)
		transitions.Inc(s.State, s.Reason)
		previous = s
	}
}
