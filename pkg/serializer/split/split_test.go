// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package split

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	metricsserializer "github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

func TestSplitPayloadsSeries(t *testing.T) {
	// Override size limits to avoid test timeouts
	prevMaxPayloadSizeCompressed := maxPayloadSizeCompressed
	maxPayloadSizeCompressed = 1024
	defer func() { maxPayloadSizeCompressed = prevMaxPayloadSizeCompressed }()

	prevMaxPayloadSizeUnCompressed := maxPayloadSizeUnCompressed
	maxPayloadSizeUnCompressed = 2 * 1024
	defer func() { maxPayloadSizeUnCompressed = prevMaxPayloadSizeUnCompressed }()

	t.Run("both compressed and uncompressed series payload under limits", func(t *testing.T) {
		testSplitPayloadsSeries(t, 2, false)
	})
	t.Run("compressed series payload over limit but uncompressed under limit", func(t *testing.T) {
		testSplitPayloadsSeries(t, 5, false)
	})
	t.Run("both compressed and uncompressed series payload over limits", func(t *testing.T) {
		testSplitPayloadsSeries(t, 8, false)
	})
	t.Run("compressed series payload under limit and uncompressed series payload over limit", func(t *testing.T) {
		testSplitPayloadsSeries(t, 8, true)
	})
}

func testSplitPayloadsSeries(t *testing.T, numPoints int, compress bool) {
	testSeries := metricsserializer.Series{}
	for i := 0; i < numPoints; i++ {
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
			Tags:     tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes"}),
		}
		testSeries = append(testSeries, &point)
	}

	payloads, err := Payloads(testSeries, compress, JSONMarshalFct)
	require.Nil(t, err)

	originalLength := len(testSeries)
	var splitSeries = []metricsserializer.Series{}
	for _, payload := range payloads {
		var s = map[string]metricsserializer.Series{}
		localPayload := payload.GetContent()

		if compress {
			localPayload, err = compression.Decompress(localPayload)
			require.Nil(t, err)
		}

		err = json.Unmarshal(localPayload, &s)
		require.Nil(t, err)
		splitSeries = append(splitSeries, s["series"])
	}

	unrolledSeries := metricsserializer.Series{}
	for _, series := range splitSeries {
		for _, s := range series {
			unrolledSeries = append(unrolledSeries, s)
		}
	}
	newLength := len(unrolledSeries)
	require.Equal(t, originalLength, newLength)
}

var result transaction.BytesPayloads

func BenchmarkSplitPayloadsSeries(b *testing.B) {
	testSeries := metricsserializer.Series{}
	for i := 0; i < 400000; i++ {
		point := metrics.Serie{
			Points: []metrics.Point{
				{Ts: 12345.0, Value: 1.2 * float64(i)},
			},
			MType:    metrics.APIGaugeType,
			Name:     fmt.Sprintf("test.metrics%d", i),
			Interval: 1,
			Host:     "localHost",
			Tags:     tagset.CompositeTagsFromSlice([]string{"tag1", "tag2:yes"}),
		}
		testSeries = append(testSeries, &point)
	}

	var r transaction.BytesPayloads
	for n := 0; n < b.N; n++ {
		// always record the result of Payloads to prevent
		// the compiler eliminating the function call.
		r, _ = Payloads(testSeries, true, JSONMarshalFct)

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
		compressedSize += len(p.GetContent())
	}
	if compressedSize > 3600000 {
		panic(fmt.Sprintf("expecting no more than 3.6 MB, got %d", compressedSize))
	}
	// always store the result to a package level variable
	// so the compiler cannot eliminate the Benchmark itself.
	result = r
}

func TestSplitPayloadsEvents(t *testing.T) {
	// Override size limits to avoid test timeouts
	prevMaxPayloadSizeCompressed := maxPayloadSizeCompressed
	maxPayloadSizeCompressed = 1024
	defer func() { maxPayloadSizeCompressed = prevMaxPayloadSizeCompressed }()

	prevMaxPayloadSizeUnCompressed := maxPayloadSizeUnCompressed
	maxPayloadSizeUnCompressed = 2 * 1024
	defer func() { maxPayloadSizeUnCompressed = prevMaxPayloadSizeUnCompressed }()

	t.Run("both compressed and uncompressed event payload under limits", func(t *testing.T) {
		testSplitPayloadsEvents(t, 2, false)
	})
	t.Run("compressed event payload over limit but uncompressed under limit", func(t *testing.T) {
		testSplitPayloadsEvents(t, 6, false)
	})
	t.Run("both compressed and uncompressed event payload over limits", func(t *testing.T) {
		testSplitPayloadsEvents(t, 15, false)
	})
	t.Run("compressed event payload under limit and uncompressed event payload over limit", func(t *testing.T) {
		testSplitPayloadsEvents(t, 15, true)
	})
}

func testSplitPayloadsEvents(t *testing.T, numPoints int, compress bool) {
	testEvent := metricsserializer.Events{}
	for i := 0; i < numPoints; i++ {
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

	payloads, err := Payloads(testEvent, compress, JSONMarshalFct)
	require.Nil(t, err)

	originalLength := len(testEvent)
	unrolledEvents := []interface{}{}
	for _, payload := range payloads {
		var s map[string]interface{}
		localPayload := payload.GetContent()
		if compress {
			localPayload, err = compression.Decompress(localPayload)
			require.Nil(t, err)
		}

		err = json.Unmarshal(localPayload, &s)
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
	// Override size limits to avoid test timeouts
	prevMaxPayloadSizeCompressed := maxPayloadSizeCompressed
	maxPayloadSizeCompressed = 1024
	defer func() { maxPayloadSizeCompressed = prevMaxPayloadSizeCompressed }()

	prevMaxPayloadSizeUnCompressed := maxPayloadSizeUnCompressed
	maxPayloadSizeUnCompressed = 2 * 1024
	defer func() { maxPayloadSizeUnCompressed = prevMaxPayloadSizeUnCompressed }()

	t.Run("both compressed and uncompressed service checks payload under limits", func(t *testing.T) {
		testSplitPayloadsServiceChecks(t, 5, false)
	})
	t.Run("compressed service checks payload over limit but uncompressed under limit", func(t *testing.T) {
		testSplitPayloadsServiceChecks(t, 10, false)
	})
	t.Run("both compressed and uncompressed service checks payload over limits", func(t *testing.T) {
		testSplitPayloadsServiceChecks(t, 20, false)
	})
	t.Run("compressed service checks payload under limit and uncompressed service checks payload over limit", func(t *testing.T) {
		testSplitPayloadsServiceChecks(t, 20, true)
	})
}

func testSplitPayloadsServiceChecks(t *testing.T, numPoints int, compress bool) {
	testServiceChecks := metricsserializer.ServiceChecks{}
	for i := 0; i < numPoints; i++ {
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

	payloads, err := Payloads(testServiceChecks, compress, JSONMarshalFct)
	require.Nil(t, err)

	originalLength := len(testServiceChecks)
	unrolledServiceChecks := []interface{}{}
	for _, payload := range payloads {
		var s []interface{}
		localPayload := payload.GetContent()
		if compress {
			localPayload, err = compression.Decompress(localPayload)
			require.Nil(t, err)
		}

		err = json.Unmarshal(localPayload, &s)
		require.Nil(t, err)

		for _, sc := range s {
			unrolledServiceChecks = append(unrolledServiceChecks, &sc)
		}
	}

	newLength := len(unrolledServiceChecks)
	require.Equal(t, originalLength, newLength)
}
