// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package metrics

import (
	"expvar"
)

var (
	// LogsExpvars contains metrics for the logs agent.
	LogsExpvars *expvar.Map
	// LogsDecoded is the total number of decoded logs
	LogsDecoded = expvar.Int{}
	// LogsProcessed is the total number of processed logs.
	LogsProcessed = expvar.Int{}
	// LogsSent is the total number of sent logs.
	LogsSent = expvar.Int{}
	// DestinationErrors is the total number of network errors.
	DestinationErrors = expvar.Int{}
	// DestinationLogsDropped is the total number of logs dropped per Destination
	DestinationLogsDropped = expvar.Map{}
	// BytesSent is the total number of sent bytes before encoding if any
	BytesSent = expvar.Int{}
	// EncodedBytesSent is the total number of sent bytes after encoding if any
	EncodedBytesSent = expvar.Int{}
	// TODO: Add LogsCollected for the total number of collected logs.
)

func init() {
	LogsExpvars = expvar.NewMap("logs-agent")
	LogsExpvars.Set("LogsDecoded", &LogsDecoded)
	LogsExpvars.Set("LogsProcessed", &LogsProcessed)
	LogsExpvars.Set("LogsSent", &LogsSent)
	LogsExpvars.Set("DestinationErrors", &DestinationErrors)
	LogsExpvars.Set("DestinationLogsDropped", &DestinationLogsDropped)
	LogsExpvars.Set("BytesSent", &BytesSent)
	LogsExpvars.Set("EncodedBytesSent", &EncodedBytesSent)
}
