// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http

import (
	"fmt"
	netebpf "github.com/DataDog/datadog-agent/pkg/network/ebpf"
	"github.com/DataDog/datadog-agent/pkg/network/ebpf/probes"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/cihub/seelog"
	"time"
	"unsafe"

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

type Telemetry struct {
	protocol string

	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	hits1XX, hits2XX, hits3XX, hits4XX, hits5XX *libtelemetry.Counter

	totalHitsPlain                                                   *libtelemetry.Counter
	totalHitsGnuTLS                                                  *libtelemetry.Counter
	totalHitsOpenSLL                                                 *libtelemetry.Counter
	totalHitsJavaTLS                                                 *libtelemetry.Counter
	totalHitsGoTLS                                                   *libtelemetry.Counter
	dropped                                                          *libtelemetry.Counter // this happens when statKeeper reaches capacity
	rejected                                                         *libtelemetry.Counter // this happens when an user-defined reject-filter matches a request
	emptyPath, unknownMethod, invalidLatency, nonPrintableCharacters *libtelemetry.Counter // this happens when the request doesn't have the expected format
	aggregations                                                     *libtelemetry.Counter

	// http2 telemetry - which should be moved its own file
	http2requests  *libtelemetry.Gauge
	http2responses *libtelemetry.Gauge
	endOfStreamEOS *libtelemetry.Gauge
	endOfStreamRST *libtelemetry.Gauge
	//strLenGraterThenFrameLoc *libtelemetry.Gauge
	//strLenTooBigMid          *libtelemetry.Gauge
	//strLenTooBigLarge        *libtelemetry.Gauge
	//frameRemainder           *libtelemetry.Gauge
	//framesInPacket           *libtelemetry.Gauge

	joiner telemetryJoiner
}

func NewTelemetry(protocol string) *Telemetry {
	metricGroup := libtelemetry.NewMetricGroup(fmt.Sprintf("usm.%s", protocol))
	metricGroupJoiner := libtelemetry.NewMetricGroup(fmt.Sprintf("usm.%s.joiner", protocol))

	return &Telemetry{
		protocol:    protocol,
		metricGroup: metricGroup,

		hits1XX:      metricGroup.NewCounter("hits", "status:1xx", libtelemetry.OptPrometheus),
		hits2XX:      metricGroup.NewCounter("hits", "status:2xx", libtelemetry.OptPrometheus),
		hits3XX:      metricGroup.NewCounter("hits", "status:3xx", libtelemetry.OptPrometheus),
		hits4XX:      metricGroup.NewCounter("hits", "status:4xx", libtelemetry.OptPrometheus),
		hits5XX:      metricGroup.NewCounter("hits", "status:5xx", libtelemetry.OptPrometheus),
		aggregations: metricGroup.NewCounter("aggregations", libtelemetry.OptPrometheus),

		// these metrics are also exported as statsd metrics
		totalHitsPlain:         metricGroup.NewCounter("total_hits", "encrypted:false", libtelemetry.OptStatsd),
		totalHitsGnuTLS:        metricGroup.NewCounter("total_hits", "encrypted:true", "tls_library:gnutls", libtelemetry.OptStatsd),
		totalHitsOpenSLL:       metricGroup.NewCounter("total_hits", "encrypted:true", "tls_library:openssl", libtelemetry.OptStatsd),
		totalHitsJavaTLS:       metricGroup.NewCounter("total_hits", "encrypted:true", "tls_library:java", libtelemetry.OptStatsd),
		totalHitsGoTLS:         metricGroup.NewCounter("total_hits", "encrypted:true", "tls_library:go", libtelemetry.OptStatsd),
		dropped:                metricGroup.NewCounter("dropped", libtelemetry.OptStatsd),
		rejected:               metricGroup.NewCounter("rejected", libtelemetry.OptStatsd),
		emptyPath:              metricGroup.NewCounter("malformed", "type:empty-path", libtelemetry.OptStatsd),
		unknownMethod:          metricGroup.NewCounter("malformed", "type:unknown-method", libtelemetry.OptStatsd),
		invalidLatency:         metricGroup.NewCounter("malformed", "type:invalid-latency", libtelemetry.OptStatsd),
		nonPrintableCharacters: metricGroup.NewCounter("malformed", "type:non-printable-char", libtelemetry.OptStatsd),

		// http2 metrics which supposed to move to the HTTP/2 package
		http2requests:  metricGroup.NewGauge("http2requests", libtelemetry.OptStatsd),
		http2responses: metricGroup.NewGauge("http2responses", libtelemetry.OptStatsd),
		endOfStreamEOS: metricGroup.NewGauge("endOfStreamEOS", libtelemetry.OptStatsd),
		endOfStreamRST: metricGroup.NewGauge("endOfStreamRST", libtelemetry.OptStatsd),
		//strLenGraterThenFrameLoc: metricGroup.NewGauge("strLenGraterThenFrameLoc", libtelemetry.OptPrometheus),
		//strLenTooBigMid:          metricGroup.NewGauge("strLenTooBigMid", libtelemetry.OptPrometheus),
		//strLenTooBigLarge:        metricGroup.NewGauge("strLenTooBigLarge", libtelemetry.OptPrometheus),
		//frameRemainder:           metricGroup.NewGauge("frameRemainder", libtelemetry.OptPrometheus),
		//framesInPacket:           metricGroup.NewGauge("framesInPacket", libtelemetry.OptPrometheus),

		joiner: telemetryJoiner{
			requests:         metricGroupJoiner.NewCounter("requests", libtelemetry.OptPrometheus),
			responses:        metricGroupJoiner.NewCounter("responses", libtelemetry.OptPrometheus),
			responsesDropped: metricGroupJoiner.NewCounter("responses_dropped", libtelemetry.OptPrometheus),
			requestJoined:    metricGroupJoiner.NewCounter("joined", libtelemetry.OptPrometheus),
			agedRequest:      metricGroupJoiner.NewCounter("aged", libtelemetry.OptPrometheus),
		},
	}
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

	t.countOSSpecific(tx)
}

func (t *Telemetry) Log() {
	log.Debugf("%s stats summary: %s", t.protocol, t.metricGroup.Summary())
}

func (t *Telemetry) getHTTP2EBPFTelemetry(mgr *manager.Manager) *netebpf.HTTP2Telemetry {
	var zero uint64
	mp, _, err := mgr.GetMap(probes.HTTP2TelemetryMap)
	if err != nil {
		log.Warnf("error retrieving http2 telemetry map: %s", err)
		return nil
	}

	http2Telemetry := &netebpf.HTTP2Telemetry{}
	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(http2Telemetry)); err != nil {
		// This can happen if we haven't initialized the telemetry object yet
		// so let's just use a trace log
		if log.ShouldLog(seelog.TraceLvl) {
			log.Tracef("error retrieving the http2 telemetry struct: %s", err)
		}
		return nil
	}
	return http2Telemetry
}

// Update should be moved to the HTTP/2 part as well
func (t *Telemetry) Update(mgr *manager.Manager) error {
	var zero uint64

	for {
		mp, _, err := mgr.GetMap(probes.HTTP2TelemetryMap)
		if err != nil {
			log.Warnf("error retrieving http2 telemetry map: %s", err)
			return nil
		}

		http2Telemetry := &netebpf.HTTP2Telemetry{}
		if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(http2Telemetry)); err != nil {
			// This can happen if we haven't initialized the telemetry object yet
			// so let's just use a trace log
			if log.ShouldLog(seelog.TraceLvl) {
				log.Tracef("error retrieving the http2 telemetry struct: %s", err)
			}
			return nil
		}

		t.http2requests.Set(int64(http2Telemetry.Request_seen))
		t.http2responses.Set(int64(http2Telemetry.Response_seen))
		t.endOfStreamEOS.Set(int64(http2Telemetry.End_of_stream_eos))
		t.endOfStreamRST.Set(int64(http2Telemetry.End_of_stream_rst))
		//t.strLenGraterThenFrameLoc.Set(int64(http2Telemetry.Str_len_greater_then_frame_loc))
		//t.frameRemainder.Set(int64(http2Telemetry.Frame_remainder))
		//t.framesInPacket.Set(int64(http2Telemetry.Max_frames_in_packet))
		//t.strLenTooBigMid.Set(int64(http2Telemetry.Str_len_too_big_mid))
		//t.strLenTooBigLarge.Set(int64(http2Telemetry.Str_len_too_big_large))

		time.Sleep(10 * time.Second)
	}
}
