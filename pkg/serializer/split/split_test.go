// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && zlib && zstd

package split

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	metricsserializer "github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/compression"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
)

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

	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}
	logger := logmock.New(t)
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {

			testEvent := metricsserializer.Events{}
			for i := 0; i < numPoints; i++ {
				event := event.Event{
					Title:          "test title",
					Text:           "test text",
					Ts:             12345,
					Host:           "test.localhost",
					Tags:           []string{"tag1", "tag2:yes"},
					AggregationKey: "test aggregation",
					SourceTypeName: "test source",
				}
				testEvent.EventsArr = append(testEvent.EventsArr, &event)
			}

			mockConfig := mock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
			payloads, err := Payloads(testEvent, compress, compressor, logger)
			require.Nil(t, err)

			originalLength := len(testEvent.EventsArr)
			unrolledEvents := []interface{}{}
			for _, payload := range payloads {
				var s map[string]interface{}
				localPayload := payload.GetContent()
				if compress {
					localPayload, err = compressor.Decompress(localPayload)
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
		})
	}
}
