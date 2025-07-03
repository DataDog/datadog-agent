// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package redis

import (
	"fmt"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// telemetry is a struct to hold the telemetry for the redis protocol
type telemetry struct {
	metricGroup *libtelemetry.MetricGroup

	commandDistribution map[CommandType]*libtelemetry.Counter
	invalidCommand      *libtelemetry.Counter
	dropped             *libtelemetry.Counter
	invalidLatency      *libtelemetry.Counter
}

// newTelemetry creates a new telemetry instance for the redis protocol
func newTelemetry() *telemetry {
	metricGroup := libtelemetry.NewMetricGroup("usm.redis")

	telem := telemetry{
		metricGroup:    metricGroup,
		dropped:        metricGroup.NewCounter("dropped", libtelemetry.OptPrometheus),
		invalidCommand: metricGroup.NewCounter("malformed", "type:invalid-command", libtelemetry.OptPrometheus),
		invalidLatency: metricGroup.NewCounter("malformed", "type:invalid-latency", libtelemetry.OptPrometheus),
	}

	telem.commandDistribution = make(map[CommandType]*libtelemetry.Counter, maxCommand)

	for command := UnknownCommand; command < maxCommand; command++ {
		telem.commandDistribution[command] = metricGroup.NewCounter("total_hits", fmt.Sprintf("command:%s", command.String()), libtelemetry.OptPrometheus)
	}

	return &telem
}

// Log logs the redis stats summary
func (t *telemetry) Log() {
	if log.ShouldLog(log.DebugLvl) {
		log.Debugf("redis stats summary: %s", t.metricGroup.Summary())
	}
}
