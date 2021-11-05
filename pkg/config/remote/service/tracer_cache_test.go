// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

import (
	"testing"
	"time"

	"gotest.tools/assert"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo"
)

func TestExpiration(t *testing.T) {
	ttl := time.Nanosecond
	tc := NewTracerCache(1000, ttl, 100*time.Nanosecond) // 10 cleanups per millisecond
	defer tc.Stop()
	tracer := &pbgo.TracerInfo{RuntimeId: "foo", ServiceName: "service", ServiceEnv: "env", ServiceVersion: "version"}
	if err := tc.TrackTracer(tracer); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * ttl)
	tc.cleanup()
	assert.Assert(t, len(tc.Tracers()) == 0, "Tracers should be empty eventually.")
}

func TestMaxTracers(t *testing.T) {
	tc := NewTracerCache(0, time.Hour, time.Hour)
	defer tc.Stop()
	tracer := &pbgo.TracerInfo{RuntimeId: "", ServiceName: "", ServiceEnv: "", ServiceVersion: ""}
	err := tc.TrackTracer(tracer)
	assert.Error(t, err, "TracerCache maxCapacity reached. Refusing to add tracer ")
}

func TestTracerTTLRefresh(t *testing.T) {
	tc := NewTracerCache(1, time.Hour, time.Hour)
	defer tc.Stop()
	tracer := &pbgo.TracerInfo{RuntimeId: "foo", ServiceName: "", ServiceEnv: "", ServiceVersion: ""}
	if err := tc.TrackTracer(tracer); err != nil {
		t.Fatal(err)
	}
	first := tc.tracerInfos["foo"].lastSeen
	time.Sleep(time.Millisecond)
	if err := tc.TrackTracer(tracer); err != nil {
		t.Fatal(err)
	}
	second := tc.tracerInfos["foo"].lastSeen
	assert.Assert(t, first != second, "LastSeen should have been updated.")
}
