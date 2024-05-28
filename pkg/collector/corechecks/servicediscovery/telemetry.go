// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"errors"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	metricTagErrorCode       = "error_code"
	metricTagServiceName     = "service_name"
	metricTagServiceLanguage = "service_language"
	metricTagServiceType     = "service_type"
)

var (
	metricFailureEvents = telemetry.NewCounterWithOpts(
		CheckName,
		"failure_events",
		[]string{metricTagErrorCode, metricTagServiceName, metricTagServiceLanguage, metricTagServiceType},
		"Number of times an error or an unexpected event happened.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
	metricDiscoveredServices = telemetry.NewGaugeWithOpts(
		CheckName,
		"discovered_services",
		[]string{},
		"Number of discovered alive services.",
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
	metricTimeToScan = telemetry.NewHistogramWithOpts(
		CheckName,
		"time_to_scan",
		[]string{},
		"Time it took to scan services",
		prometheus.DefBuckets,
		telemetry.Options{NoDoubleUnderscoreSep: true},
	)
)

func telemetryFromError(err error) {
	var codeErr errWithCode
	if errors.As(err, &codeErr) {
		log.Debugf("sending telemetry for error: %v", err)
		svc := serviceMetadata{}
		if codeErr.svc != nil {
			svc = *codeErr.svc
		}
		tags := []string{string(codeErr.Code()), svc.Name, svc.Language, svc.Type}
		metricFailureEvents.Inc(tags...)
	}
}
