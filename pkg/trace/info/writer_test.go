// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package info

import (
	"testing"
)

func TestPublishTraceWriterInfo(t *testing.T) {
	traceWriterInfo = TraceWriterInfo{
		// do not use field names here, to ensure we cover all fields
		atom(1),
		atom(2),
		atom(3),
		atom(4),
		atom(5),
		atom(6),
		atom(7),
		atom(8),
		atom(9),
	}

	testExpvarPublish(t, publishTraceWriterInfo,
		map[string]interface{}{
			// all JSON numbers are floats, so the results come back as floats
			"Payloads":          1.0,
			"Traces":            2.0,
			"Events":            3.0,
			"Spans":             4.0,
			"Errors":            5.0,
			"Retries":           6.0,
			"Bytes":             7.0,
			"BytesUncompressed": 8.0,
			"SingleMaxSize":     9.0,
		})
}

func TestPublishStatsWriterInfo(t *testing.T) {
	statsWriterInfo = StatsWriterInfo{
		// do not use field names here, to ensure we cover all fields
		atom(1),
		atom(2),
		atom(3),
		atom(4),
		atom(5),
		atom(6),
		atom(7),
		atom(8),
	}

	testExpvarPublish(t, publishStatsWriterInfo,
		map[string]interface{}{
			// all JSON numbers are floats, so the results come back as floats
			"Payloads":       1.0,
			"ClientPayloads": 2.0,
			"StatsBuckets":   3.0,
			"StatsEntries":   4.0,
			"Errors":         5.0,
			"Retries":        6.0,
			"Splits":         7.0,
			"Bytes":          8.0,
		})
}

func TestPublishRateByService(t *testing.T) {
	rateByService = map[string]float64{"foo": 123.0}

	testExpvarPublish(t, publishRateByService,
		map[string]interface{}{
			"foo": 123.0,
		})
}
