// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package telemetry

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	telemetrymock "github.com/DataDog/datadog-agent/comp/core/telemetry/mock"
)

func TestRecordForwarderHTTPError(t *testing.T) {
	tel := telemetrymock.New(t)
	c := NewForwarderHTTPErrors(tel)

	for _, status := range []int{200, 204, 301, 399, 400, 404, 404, 503} {
		RecordForwarderHTTPError(c, "fwd", "https://intake.example", "ep", status)
	}

	metrics, err := tel.GetCountMetric("forwarder", "http_errors")
	require.NoError(t, err)
	got := map[string]float64{}
	for _, m := range metrics {
		assert.Equal(t, "https://intake.example", m.Tags()["domain"])
		assert.Equal(t, "ep", m.Tags()["endpoint"])
		assert.Equal(t, "fwd", m.Tags()["forwarder"])
		got[m.Tags()["code"]] = m.Value()
	}
	// Only status >= 400, with the exact code.
	assert.Equal(t, map[string]float64{"400": 1, "404": 2, "503": 1}, got)
}

func TestForwarderHTTPErrorsIsShared(t *testing.T) {
	series := ForwarderHTTPErrors.WithValues("https://shared.example", "ep", "404", "fwd")
	before := series.Get()
	RecordForwarderHTTPError(ForwarderHTTPErrors, "fwd", "https://shared.example", "ep", 404)
	assert.Equal(t, before+1, series.Get())
}
