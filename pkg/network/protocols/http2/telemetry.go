// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build (windows && npm) || linux_bpf

package http2

import (
	"fmt"

	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

type Http2KernelTelemetry struct {
	// metricGroup is used here mostly for building the log message below
	metricGroup *libtelemetry.MetricGroup

	// http2requests          http2 requests seen
	// http2requests          http2 responses seen
	// endOfStreamEOS         END_OF_STREAM flags seen
	// endOfStreamRST         RST seen
	// largePathInDelta       Amount of path size between 160-180 bytes
	// largePathOutsideDelta  Amount of path size between bigger than 180 bytes

	http2requests         *libtelemetry.Gauge
	http2responses        *libtelemetry.Gauge
	endOfStreamEOS        *libtelemetry.Gauge
	endOfStreamRST        *libtelemetry.Gauge
	largePathInDelta      *libtelemetry.Gauge
	largePathOutsideDelta *libtelemetry.Gauge
	//strLenGraterThenFrameLoc *libtelemetry.Gauge
	//frameRemainder           *libtelemetry.Gauge
	//framesInPacket           *libtelemetry.Gauge

}

// NewHTTP2KernelTelemetry may be moved to the HTTP/2 tlemetry file
func NewHTTP2KernelTelemetry(protocol string) *Http2KernelTelemetry {
	metricGroup := libtelemetry.NewMetricGroup(fmt.Sprintf("usm.%s", protocol))
	return &Http2KernelTelemetry{
		metricGroup: metricGroup,

		http2requests:  metricGroup.NewGauge("http2requests", libtelemetry.OptStatsd),
		http2responses: metricGroup.NewGauge("http2responses", libtelemetry.OptStatsd),
		endOfStreamEOS: metricGroup.NewGauge("endOfStreamEOS", libtelemetry.OptStatsd),
		endOfStreamRST: metricGroup.NewGauge("endOfStreamRST", libtelemetry.OptStatsd),
		//strLenGraterThenFrameLoc: metricGroup.NewGauge("strLenGraterThenFrameLoc", libtelemetry.OptPrometheus),
		largePathInDelta:      metricGroup.NewGauge("largePathInDelta", libtelemetry.OptStatsd),
		largePathOutsideDelta: metricGroup.NewGauge("largePathOutsideDelta", libtelemetry.OptStatsd),
		//frameRemainder:           metricGroup.NewGauge("frameRemainder", libtelemetry.OptPrometheus),
		//framesInPacket:           metricGroup.NewGauge("framesInPacket", libtelemetry.OptPrometheus),
	}
}

//func (t *Telemetry) getHTTP2EBPFTelemetry(mgr *manager.Manager) *HTTP2Telemetry {
//	var zero uint64
//	mp, _, err := mgr.GetMap(probes.HTTP2TelemetryMap)
//	if err != nil {
//		log.Warnf("error retrieving http2 telemetry map: %s", err)
//		return nil
//	}
//
//	Http2KernelTelemetry := &HTTP2Telemetry{}
//	if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(Http2KernelTelemetry)); err != nil {
//		// This can happen if we haven't initialized the telemetry object yet
//		// so let's just use a trace log
//		if log.ShouldLog(seelog.TraceLvl) {
//			log.Tracef("error retrieving the http2 telemetry struct: %s", err)
//		}
//		return nil
//	}
//	return Http2KernelTelemetry
//}
//
//// Update should be moved to the HTTP/2 part as well
//func (t *Telemetry) Update(mgr *manager.Manager) error {
//	var zero uint64
//
//	for {
//		mp, _, err := mgr.GetMap(probes.HTTP2TelemetryMap)
//		if err != nil {
//			log.Warnf("error retrieving http2 telemetry map: %s", err)
//			return nil
//		}
//
//		Http2KernelTelemetry := &HTTP2Telemetry{}
//		if err := mp.Lookup(unsafe.Pointer(&zero), unsafe.Pointer(Http2KernelTelemetry)); err != nil {
//			// This can happen if we haven't initialized the telemetry object yet
//			// so let's just use a trace log
//			if log.ShouldLog(seelog.TraceLvl) {
//				log.Tracef("error retrieving the http2 telemetry struct: %s", err)
//			}
//			return nil
//		}
//
//		//t.http2requests.Set(int64(Http2KernelTelemetry.Request_seen))
//		//t.http2responses.Set(int64(Http2KernelTelemetry.Response_seen))
//		t.endOfStreamEOS.Set(int64(Http2KernelTelemetry.End_of_stream_eos))
//		t.endOfStreamRST.Set(int64(Http2KernelTelemetry.End_of_stream_rst))
//		//t.strLenGraterThenFrameLoc.Set(int64(Http2KernelTelemetry.Str_len_greater_then_frame_loc))
//		//t.frameRemainder.Set(int64(Http2KernelTelemetry.Frame_remainder))
//		//t.framesInPacket.Set(int64(Http2KernelTelemetry.Max_frames_in_packet))
//		t.largePathInDelta.Set(int64(Http2KernelTelemetry.Large_path_in_delta))
//		t.largePathOutsideDelta.Set(int64(Http2KernelTelemetry.Large_path_outside_delta))
//
//		time.Sleep(10 * time.Second)
//	}
//}
//
//func (t *Telemetry) Count(tx http.Transaction) {
//	statusClass := (tx.StatusCode() / 100) * 100
//	switch statusClass {
//	case 100:
//		t.hits1XX.Add(1)
//	case 200:
//		t.hits2XX.Add(1)
//	case 300:
//		t.hits3XX.Add(1)
//	case 400:
//		t.hits4XX.Add(1)
//	case 500:
//		t.hits5XX.Add(1)
//	}
//
//	t.countOSSpecific(tx)
//}
