// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package split

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/metrics"
)

func TestSplitPayloadsSeries(t *testing.T) {
	testSeries := metrics.Series{}
	for i := 0; i < 30000; i++ {
		point := metrics.Serie{
			Points: []metrics.Point{
				{Ts: 12345.0, Value: float64(21.21)},
				{Ts: 67890.0, Value: float64(12.12)},
				{Ts: 2222.0, Value: float64(22.12)},
				{Ts: 333.0, Value: float64(32.12)},
				{Ts: 444444.0, Value: float64(42.12)},
				{Ts: 882787.0, Value: float64(52.12)},
				{Ts: 99990.0, Value: float64(62.12)},
				{Ts: 121212.0, Value: float64(72.12)},
				{Ts: 222227.0, Value: float64(82.12)},
				{Ts: 808080.0, Value: float64(92.12)},
				{Ts: 9090.0, Value: float64(13.12)},
			},
			MType:    metrics.APIGaugeType,
			Name:     fmt.Sprintf("test.metrics%d", i),
			Interval: 1,
			Host:     "localHost",
			Tags:     []string{"tag1", "tag2:yes"},
		}
		testSeries = append(testSeries, &point)
	}

	originalLength := len(testSeries)
	payloads, err := Payloads(testSeries, false, MarshalJSON)
	require.Nil(t, err)
	var splitSeries = []metrics.Series{}
	for _, payload := range payloads {
		var s = map[string]metrics.Series{}
		err = json.Unmarshal(*payload, &s)
		require.Nil(t, err)
		splitSeries = append(splitSeries, s["series"])
	}

	unrolledSeries := metrics.Series{}
	for _, series := range splitSeries {
		for _, s := range series {
			unrolledSeries = append(unrolledSeries, s)
		}
	}

	newLength := len(unrolledSeries)
	require.Equal(t, originalLength, newLength)
}

var result forwarder.Payloads

func BenchmarkSplitPayloadsSeries(b *testing.B) {
	testSeries := metrics.Series{}
	for i := 0; i < 400000; i++ {
		point := metrics.Serie{
			Points: []metrics.Point{
				{Ts: 12345.0, Value: 1.2 * float64(i)},
			},
			MType:    metrics.APIGaugeType,
			Name:     fmt.Sprintf("test.metrics%d", i),
			Interval: 1,
			Host:     "localHost",
			Tags:     []string{"tag1", "tag2:yes"},
		}
		testSeries = append(testSeries, &point)
	}

	var r forwarder.Payloads
	for n := 0; n < b.N; n++ {
		// always record the result of Payloads to prevent
		// the compiler eliminating the function call.
		r, _ = Payloads(testSeries, true, MarshalJSON)

	}
	// ensure we actually had to split
	if len(r) < 2 {
		panic(fmt.Sprintf("expecting more than one payload, got %d", len(r)))
	}
	// test the compressed size
	var compressedSize int
	for _, p := range r {
		if p == nil {
			continue
		}
		compressedSize += len([]byte(*p))
	}
	if compressedSize > 3600000 {
		panic(fmt.Sprintf("expecting no more than 3.6 MB, got %d", compressedSize))
	}
	// always store the result to a package level variable
	// so the compiler cannot eliminate the Benchmark itself.
	result = r
}

func TestSplitPayloadsEvents(t *testing.T) {
	testEvent := metrics.Events{}
	for i := 0; i < 30000; i++ {
		event := metrics.Event{
			Title:          "test title",
			Text:           "test text",
			Ts:             12345,
			Host:           "test.localhost",
			Tags:           []string{"tag1", "tag2:yes"},
			AggregationKey: "test aggregation",
			SourceTypeName: "test source",
		}
		testEvent = append(testEvent, &event)
	}

	originalLength := len(testEvent)
	payloads, err := Payloads(testEvent, false, MarshalJSON)
	require.Nil(t, err)
	unrolledEvents := []interface{}{}
	for _, payload := range payloads {
		var s map[string]interface{}
		err = json.Unmarshal(*payload, &s)
		require.Nil(t, err)
		for _, events := range s["events"].(map[string]interface{}) {
			for _, evt := range events.([]interface{}) {
				unrolledEvents = append(unrolledEvents, &evt)
			}
		}
	}

	newLength := len(unrolledEvents)
	require.Equal(t, originalLength, newLength)
}

func TestSplitPayloadsServiceChecks(t *testing.T) {
	testServiceChecks := metrics.ServiceChecks{}
	for i := 0; i < 30000; i++ {
		sc := metrics.ServiceCheck{
			CheckName: "test.check",
			Host:      "test.localhost",
			Ts:        1000,
			Status:    metrics.ServiceCheckOK,
			Message:   "this is fine",
			Tags:      []string{"tag1", "tag2:yes"},
		}
		testServiceChecks = append(testServiceChecks, &sc)
	}

	originalLength := len(testServiceChecks)
	payloads, err := Payloads(testServiceChecks, false, MarshalJSON)
	require.Nil(t, err)
	unrolledServiceChecks := []interface{}{}
	for _, payload := range payloads {
		var s []interface{}
		err = json.Unmarshal(*payload, &s)
		require.Nil(t, err)
		for _, sc := range s {
			unrolledServiceChecks = append(unrolledServiceChecks, &sc)
		}
	}

	newLength := len(testServiceChecks)
	require.Equal(t, originalLength, newLength)
}
