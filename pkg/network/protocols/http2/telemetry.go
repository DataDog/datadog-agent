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

type kernelTelemetry struct {
	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	// http2requests             Count of HTTP/2 requests seen
	// http2responses            Count of HTTP/2 responses seen

	http2requests  *libtelemetry.Gauge
	http2responses *libtelemetry.Gauge
	endOfStreamEOS *libtelemetry.Gauge
}

// newHTTP2KernelTelemetry hold HTTP/2 kernel metrics.
func newHTTP2KernelTelemetry(protocol string) *kernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup(fmt.Sprintf("usm.%s", protocol))
	return &kernelTelemetry{
		metricGroup: metricGroup,

		// todo: changed it from OptStatsd to OptPrometheus
		http2requests:  metricGroup.NewGauge("http2requests", libtelemetry.OptStatsd),
		endOfStreamEOS: metricGroup.NewGauge("endOfStreamEOS", libtelemetry.OptStatsd),
		http2responses: metricGroup.NewGauge("http2responses", libtelemetry.OptStatsd)}
}

func (t *kernelTelemetry) Log() {
	log.Debugf("http2 kernel telemetry summary: %s", t.metricGroup.Summary())
}
