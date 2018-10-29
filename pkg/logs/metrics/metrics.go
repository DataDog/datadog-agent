// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package metrics

import (
	"expvar"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/status"
)

var (
	logsExpvars *expvar.Map
	// LogsDecoded is the total number of decoded logs
	LogsDecoded = expvar.Int{}
	// LogsProcessed is the total number of processed logs.
	LogsProcessed = expvar.Int{}
	// LogsSent is the total number of sent logs.
	LogsSent = expvar.Int{}
	// DestinationErrors is the total number of network errors.
	DestinationErrors = expvar.Int{}
	// TODO: Add LogsCollected for the total number of collected logs.
)

func init() {
	logsExpvars = expvar.NewMap("logs-agent")
	logsExpvars.Set("LogsDecoded", &LogsDecoded)
	logsExpvars.Set("LogsProcessed", &LogsProcessed)
	logsExpvars.Set("LogsSent", &LogsSent)
	logsExpvars.Set("DestinationErrors", &DestinationErrors)
	logsExpvars.Set("Warnings", expvar.Func(func() interface{} {
		return strings.Join(status.Get().Messages, ", ")
	}))
	logsExpvars.Set("IsRunning", expvar.Func(func() interface{} {
		return status.Get().IsRunning
	}))
}
