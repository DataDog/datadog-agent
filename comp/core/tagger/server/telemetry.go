// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
)

const subsystem = "tagger"

type telemetryStore struct {
	// ServerStreamErrors tracks how many errors happened when streaming out
	// tagger events.
	ServerStreamErrors telemetry.Counter
}

func newTelemetryStore(telemetryComp telemetry.Component) *telemetryStore {
	return &telemetryStore{
		ServerStreamErrors: telemetryComp.NewCounterWithOpts(
			subsystem,
			"server_stream_errors",
			[]string{},
			"Errors when streaming out tagger events",
			telemetry.Options{NoDoubleUnderscoreSep: true},
		),
	}
}
