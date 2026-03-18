// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf || darwin

package dns

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

const (
	dnsCacheExpirationPeriod = 1 * time.Minute
	dnsCacheSize             = 100000
	dnsModuleName            = "network_tracer__dns"
)

// snooperTelemetry holds DNS packet processing counters shared across all DNS monitor implementations.
var snooperTelemetry = struct {
	decodingErrors *telemetry.StatCounterWrapper
	truncatedPkts  *telemetry.StatCounterWrapper
	queries        *telemetry.StatCounterWrapper
	successes      *telemetry.StatCounterWrapper
	errors         *telemetry.StatCounterWrapper
}{
	telemetry.NewStatCounterWrapper(dnsModuleName, "decoding_errors", []string{}, "Counter measuring the number of decoding errors while processing packets"),
	telemetry.NewStatCounterWrapper(dnsModuleName, "truncated_pkts", []string{}, "Counter measuring the number of truncated packets while processing"),
	// DNS telemetry, values calculated *till* the last tick in pollStats
	telemetry.NewStatCounterWrapper(dnsModuleName, "queries", []string{}, "Counter measuring the number of packets that are DNS queries in processed packets"),
	telemetry.NewStatCounterWrapper(dnsModuleName, "successes", []string{}, "Counter measuring the number of successful DNS responses in processed packets"),
	telemetry.NewStatCounterWrapper(dnsModuleName, "errors", []string{}, "Counter measuring the number of failed DNS responses in processed packets"),
}
