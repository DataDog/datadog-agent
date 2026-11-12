// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package telemetry

import (
	"context"
	"net/http"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

type unreachableIntake struct {
	requests chan struct{}
}

func (u *unreachableIntake) RoundTrip(r *http.Request) (*http.Response, error) {
	u.requests <- struct{}{}
	<-r.Context().Done()
	return nil, r.Context().Err()
}

func newTelemetryWithUnreachableIntake(opts ...Option) (*Telemetry, *unreachableIntake) {
	intake := &unreachableIntake{requests: make(chan struct{}, 10)}
	return newTelemetry(&http.Client{Transport: intake}, "api", "datadoghq.com", "test-service", opts...), intake
}

func finishTestSpan() {
	span, _ := StartSpanFromContext(context.Background(), "stop-test")
	span.Finish(nil)
}

func TestStopWithoutTimeoutWaitsForSendTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		telem, intake := newTelemetryWithUnreachableIntake()
		telem.Start()
		finishTestSpan()

		start := time.Now()
		telem.Stop()

		assert.Len(t, intake.requests, 1)
		assert.Equal(t, defaultSendPayloadTimeout, time.Since(start))
	})
}

func TestStopFlushTimeoutBoundsFinalFlush(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const timeout = 2 * time.Second
		telem, intake := newTelemetryWithUnreachableIntake(WithStopFlushTimeout(timeout))
		telem.Start()
		finishTestSpan()

		start := time.Now()
		telem.Stop()

		assert.Len(t, intake.requests, 1)
		assert.Equal(t, timeout, time.Since(start))
	})
}

func TestStopFlushTimeoutBoundsInFlightPeriodicSend(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const timeout = 2 * time.Second
		telem, intake := newTelemetryWithUnreachableIntake(WithStopFlushTimeout(timeout))
		telem.Start()
		finishTestSpan()

		time.Sleep(time.Minute)
		synctest.Wait()
		assert.Len(t, intake.requests, 1, "the periodic flush should be waiting on the intake")

		start := time.Now()
		telem.Stop()

		assert.Equal(t, timeout, time.Since(start))
	})
}
