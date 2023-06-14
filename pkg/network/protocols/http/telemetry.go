// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"strconv"
	"time"

	"go.uber.org/atomic"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
	pkgtelemetry "github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var telem = struct {
	hits, dropped, rejected, malformed pkgtelemetry.Counter
	aggregations                       pkgtelemetry.Counter
}{
	pkgtelemetry.NewCounter("usm__http", "hits", []string{"protocol", "status_class"}, ""),
	pkgtelemetry.NewCounter("usm__http", "dropped", []string{"protocol"}, ""),
	pkgtelemetry.NewCounter("usm__http", "rejected", []string{"protocol"}, ""),
	pkgtelemetry.NewCounter("usm__http", "malformed", []string{"protocol"}, ""),
	pkgtelemetry.NewCounter("usm__http", "aggregations", []string{"protocol"}, ""),
}

type Telemetry struct {
	proto                                       string
	LastCheck                                   *atomic.Int64
	hits1XX, hits2XX, hits3XX, hits4XX, hits5XX *libtelemetry.Metric

	totalHits    *libtelemetry.Metric
	dropped      *libtelemetry.Metric // this happens when statKeeper reaches capacity
	rejected     *libtelemetry.Metric // this happens when an user-defined reject-filter matches a request
	malformed    *libtelemetry.Metric // this happens when the request doesn't have the expected format
	aggregations *libtelemetry.Metric
}

func NewTelemetry(proto string) *Telemetry {
	metricGroup := libtelemetry.NewMetricGroup(
		"usm.http",
		libtelemetry.OptExpvar,
		libtelemetry.OptMonotonic,
	)

	t := &Telemetry{
		proto:        proto,
		LastCheck:    atomic.NewInt64(time.Now().Unix()),
		hits1XX:      metricGroup.NewMetric("hits1xx"),
		hits2XX:      metricGroup.NewMetric("hits2xx"),
		hits3XX:      metricGroup.NewMetric("hits3xx"),
		hits4XX:      metricGroup.NewMetric("hits4xx"),
		hits5XX:      metricGroup.NewMetric("hits5xx"),
		aggregations: metricGroup.NewMetric("aggregations"),

		// these metrics are also exported as statsd metrics
		totalHits: metricGroup.NewMetric("total_hits", libtelemetry.OptStatsd, libtelemetry.OptPayloadTelemetry),
		dropped:   metricGroup.NewMetric("dropped", libtelemetry.OptStatsd),
		rejected:  metricGroup.NewMetric("rejected", libtelemetry.OptStatsd),
		malformed: metricGroup.NewMetric("malformed", libtelemetry.OptStatsd),
	}

	t.LastCheck.Store(time.Now().Unix())

	return t
}

func (t *Telemetry) Count(tx Transaction) {
	statusClass := (tx.StatusCode() / 100) * 100
	switch statusClass {
	case 100:
		t.hits1XX.Add(1)
	case 200:
		t.hits2XX.Add(1)
	case 300:
		t.hits3XX.Add(1)
	case 400:
		t.hits4XX.Add(1)
	case 500:
		t.hits5XX.Add(1)
	}
	t.totalHits.Add(1)
	telem.hits.Inc(t.proto, strconv.Itoa(int(statusClass)))
}

func (t *Telemetry) Log() {
	now := time.Now().Unix()

	if t.LastCheck.Load() == 0 {
		t.LastCheck.Store(now)
		return
	}
	totalRequests := t.totalHits.Delta()
	dropped := t.dropped.Delta()
	rejected := t.rejected.Delta()
	malformed := t.malformed.Delta()
	aggregations := t.aggregations.Delta()
	elapsed := now - t.LastCheck.Load()
	t.LastCheck.Store(now)

	log.Debugf(
		"http stats summary: requests_processed=%d(%.2f/s) requests_dropped=%d(%.2f/s) requests_rejected=%d(%.2f/s) requests_malformed=%d(%.2f/s) aggregations=%d",
		totalRequests,
		float64(totalRequests)/float64(elapsed),
		dropped,
		float64(dropped)/float64(elapsed),
		rejected,
		float64(rejected)/float64(elapsed),
		malformed,
		float64(malformed)/float64(elapsed),
		aggregations,
	)
}
