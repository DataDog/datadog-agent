// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package aggregator

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAddFlushTimeReportsTelemetry(t *testing.T) {
	newFlushTimeStats("TestFlushTime")

	addFlushTime("TestFlushTime", 4200)
	require.Equal(t, 4200.0, tlmFlushTime.WithValues("TestFlushTime").Get())

	// The gauge reports the most recent flush, not an accumulation.
	addFlushTime("TestFlushTime", 17)
	require.Equal(t, 17.0, tlmFlushTime.WithValues("TestFlushTime").Get())
}

func TestAddFlushCountReportsTelemetry(t *testing.T) {
	newFlushCountStats("TestFlushCount")

	addFlushCount("TestFlushCount", 12)
	require.Equal(t, 12.0, tlmFlushCount.WithValues("TestFlushCount").Get())

	addFlushCount("TestFlushCount", 0)
	require.Equal(t, 0.0, tlmFlushCount.WithValues("TestFlushCount").Get())
}
