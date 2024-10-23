// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"fmt"

	"github.com/cihub/seelog"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type telemetryJoiner struct {
	// requests          orphan requests
	// responses         orphan responses
	// responsesDropped  responses dropped as older than request
	// requestJoined     joined request and response
	// agedRequest       aged requests dropped

	requests         *libtelemetry.Counter
	responses        *libtelemetry.Counter
	responsesDropped *libtelemetry.Counter
	requestJoined    *libtelemetry.Counter
	agedRequest      *libtelemetry.Counter
}

// Telemetry is used to collect telemetry for HTTP and HTTPS traffic.
type Telemetry struct {
	protocol string

	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	hits1XX, hits2XX, hits3XX, hits4XX, hits5XX *TLSCounter

	dropped                                                          *libtelemetry.Counter // this happens when statKeeper reaches capacity
	rejected                                                         *libtelemetry.Counter // this happens when an user-defined reject-filter matches a request
	emptyPath, unknownMethod, invalidLatency, nonPrintableCharacters *libtelemetry.Counter // this happens when the request doesn't have the expected format
	aggregations                                                     *libtelemetry.Counter

	joiner telemetryJoiner
}

// NewTelemetry returns a new Telemetry.
func NewTelemetry(protocol string) *Telemetry {
	metricGroup := libtelemetry.NewMetricGroup(fmt.Sprintf("usm.%s", protocol))
	metricGroupJoiner := libtelemetry.NewMetricGroup(fmt.Sprintf("usm.%s.joiner", protocol))

	return &Telemetry{
		protocol:     protocol,
		metricGroup:  metricGroup,
		aggregations: metricGroup.NewCounter("aggregations", libtelemetry.OptPrometheus),

		// these metrics are also exported as statsd metrics
		hits1XX:                NewTLSCounter(metricGroup, "total_hits", "status:1xx", libtelemetry.OptStatsd),
		hits2XX:                NewTLSCounter(metricGroup, "total_hits", "status:2xx", libtelemetry.OptStatsd),
		hits3XX:                NewTLSCounter(metricGroup, "total_hits", "status:3xx", libtelemetry.OptStatsd),
		hits4XX:                NewTLSCounter(metricGroup, "total_hits", "status:4xx", libtelemetry.OptStatsd),
		hits5XX:                NewTLSCounter(metricGroup, "total_hits", "status:5xx", libtelemetry.OptStatsd),
		dropped:                metricGroup.NewCounter("dropped", libtelemetry.OptStatsd),
		rejected:               metricGroup.NewCounter("rejected", libtelemetry.OptStatsd),
		emptyPath:              metricGroup.NewCounter("malformed", "type:empty-path", libtelemetry.OptStatsd),
		unknownMethod:          metricGroup.NewCounter("malformed", "type:unknown-method", libtelemetry.OptStatsd),
		invalidLatency:         metricGroup.NewCounter("malformed", "type:invalid-latency", libtelemetry.OptStatsd),
		nonPrintableCharacters: metricGroup.NewCounter("malformed", "type:non-printable-char", libtelemetry.OptStatsd),

		joiner: telemetryJoiner{
			requests:         metricGroupJoiner.NewCounter("requests", libtelemetry.OptPrometheus),
			responses:        metricGroupJoiner.NewCounter("responses", libtelemetry.OptPrometheus),
			responsesDropped: metricGroupJoiner.NewCounter("responses_dropped", libtelemetry.OptPrometheus),
			requestJoined:    metricGroupJoiner.NewCounter("joined", libtelemetry.OptPrometheus),
			agedRequest:      metricGroupJoiner.NewCounter("aged", libtelemetry.OptPrometheus),
		},
	}
}

// Count counts a transaction.
func (t *Telemetry) Count(tx Transaction) {
	statusClass := (tx.StatusCode() / 100) * 100
	switch statusClass {
	case 100:
		t.hits1XX.Add(tx)
	case 200:
		t.hits2XX.Add(tx)
	case 300:
		t.hits3XX.Add(tx)
	case 400:
		t.hits4XX.Add(tx)
	case 500:
		t.hits5XX.Add(tx)
	}
}

// Log logs the telemetry.
func (t *Telemetry) Log() {
	if log.ShouldLog(seelog.DebugLvl) {
		log.Debugf("%s stats summary: %s", t.protocol, t.metricGroup.Summary())
	}
}
