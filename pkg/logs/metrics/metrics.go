// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package metrics

import (
	"expvar"
)

var (
	// LogsExpvars contains metrics for the logs agent.
	LogsExpvars *expvar.Map
	// LogsCollected is the total number of collected logs.
	LogsCollected = expvar.Int{}
	// LogsProcessed is the total number of processed logs.
	LogsProcessed = expvar.Int{}
	// LogsSent is the total number of sent logs.
	LogsSent = expvar.Int{}
	// LogsCommitted is the total number of committed logs.
	LogsCommitted = expvar.Int{}
	// DestinationErrors is the total number of network errors.
	DestinationErrors = expvar.Int{}
)

func init() {
	LogsExpvars = expvar.NewMap("logs-agent")
	LogsExpvars.Set("LogsCollected", &LogsCollected)
	LogsExpvars.Set("LogsProcessed", &LogsProcessed)
	LogsExpvars.Set("LogsSent", &LogsSent)
	LogsExpvars.Set("LogsCommitted", &LogsCommitted)
	LogsExpvars.Set("DestinationErrors", &DestinationErrors)
}
