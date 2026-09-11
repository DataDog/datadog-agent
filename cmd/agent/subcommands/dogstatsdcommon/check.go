// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package dogstatsdcommon provides shared helpers for DogStatsD-related CLI
// subcommands.
package dogstatsdcommon

import (
	"errors"

	cconfig "github.com/DataDog/datadog-agent/comp/core/config"
	dsdconfig "github.com/DataDog/datadog-agent/comp/dogstatsd/config"
)

// ErrDataPlaneOwnsDogstatsd is returned when a DogStatsD diagnostic command is
// invoked against the Core Agent while the Agent Data Plane is configured to
// handle DogStatsD traffic.
//
// In that mode the Core Agent's DogStatsD pipeline is dormant: its IPC endpoints
// are still registered but operate on empty state, so the commands would either
// error out or silently produce empty results. The user must run the equivalent
// commands against the agent-data-plane binary instead.
var ErrDataPlaneOwnsDogstatsd = errors.New(
	"DogStatsD traffic is being served by the Agent Data Plane. " +
		"Run DogStatsD diagnostic commands directly through either the " +
		"agent-data-plane binary or its corresponding container.",
)

// CheckDataPlaneOwnsDogstatsd returns ErrDataPlaneOwnsDogstatsd when the Core
// Agent is configured to delegate DogStatsD to the Agent Data Plane. Callers
// should invoke this at the top of a DogStatsD diagnostic command's execution
// function (after the fx app has loaded the configuration) and return the error
// verbatim.
func CheckDataPlaneOwnsDogstatsd(config cconfig.Component) error {
	if dsdconfig.NewConfig(config).EnabledDataPlane() {
		return ErrDataPlaneOwnsDogstatsd
	}
	return nil
}
