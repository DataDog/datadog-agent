// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http2

import (
	"fmt"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type KernelTelemetry struct {
	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	// http2requests            http2 requests seen
	// http2requests            http2 responses seen
	// endOfStreamEOS           END_OF_STREAM flags seen
	// endOfStreamRST           RST seen
	// largePathInDelta         Amount of path size between 160-180 bytes
	// largePathOutsideDelta    Amount of path size between bigger than 180 bytes
	// strLenGraterThenFrameLoc Amount of times we did not manage to get the path due to the fact we reached to the end of the frame.
	// frameRemainder		    Amount of frames that were sent over more than one frame.

	http2requests            *libtelemetry.Gauge
	http2responses           *libtelemetry.Gauge
	endOfStreamEOS           *libtelemetry.Gauge
	endOfStreamRST           *libtelemetry.Gauge
	largePathInDelta         *libtelemetry.Gauge
	largePathOutsideDelta    *libtelemetry.Gauge
	strLenGraterThenFrameLoc *libtelemetry.Gauge
	frameRemainder           *libtelemetry.Gauge
}

// NewHTTP2KernelTelemetry hold HTTP/2 kernel metrics.
func NewHTTP2KernelTelemetry(protocol string) *KernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup(fmt.Sprintf("usm.%s", protocol))
	return &KernelTelemetry{
		metricGroup: metricGroup,

		// todo: changed it from OptStatsd to OptPrometheus
		http2requests:            metricGroup.NewGauge("http2requests", libtelemetry.OptStatsd),
		http2responses:           metricGroup.NewGauge("http2responses", libtelemetry.OptStatsd),
		endOfStreamEOS:           metricGroup.NewGauge("endOfStreamEOS", libtelemetry.OptStatsd),
		endOfStreamRST:           metricGroup.NewGauge("endOfStreamRST", libtelemetry.OptStatsd),
		strLenGraterThenFrameLoc: metricGroup.NewGauge("strLenGraterThenFrameLoc", libtelemetry.OptStatsd),
		largePathInDelta:         metricGroup.NewGauge("largePathInDelta", libtelemetry.OptStatsd),
		largePathOutsideDelta:    metricGroup.NewGauge("largePathOutsideDelta", libtelemetry.OptStatsd),
		frameRemainder:           metricGroup.NewGauge("frameRemainder", libtelemetry.OptStatsd),
	}
}

func (t *KernelTelemetry) Log() {
	log.Debugf("http2 kernel telemetry summary: %s", t.metricGroup.Summary())
}
