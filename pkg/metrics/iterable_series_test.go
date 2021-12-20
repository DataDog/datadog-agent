// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metrics

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIterableSeries(t *testing.T) {
	iterableSeries := NewIterableSeries(func(*Serie) {}, 1)
	done := make(chan struct{})
	var descritions []string
	go func() {
		defer iterableSeries.IterationStopped()
		for iterableSeries.MoveNext() {
			descritions = append(descritions, iterableSeries.DescribeCurrentItem())
		}
		close(done)
	}()
	iterableSeries.Append(&Serie{Name: "serie1"})
	iterableSeries.Append(&Serie{Name: "serie2"})
	iterableSeries.Append(&Serie{Name: "serie3"})
	iterableSeries.SenderStopped()
	<-done
	r := require.New(t)
	r.Len(descritions, 3)
	r.True(strings.Contains(descritions[0], "serie1"))
	r.True(strings.Contains(descritions[1], "serie2"))
	r.True(strings.Contains(descritions[2], "serie3"))
}

func TestIterableSeriesCallback(t *testing.T) {
	var series Series
	callback := func(s *Serie) { series = append(series, s) }
	iterableSeries := NewIterableSeries(callback, 10)
	iterableSeries.Append(&Serie{Name: "serie1"})
	iterableSeries.Append(&Serie{Name: "serie2"})

	r := require.New(t)
	r.Equal(uint64(2), iterableSeries.SeriesCount())
	r.Len(series, 2)
	r.Equal("serie1", series[0].Name)
	r.Equal("serie2", series[1].Name)
}

func TestIterableSeriesReceiverStopped(t *testing.T) {
	iterableSeries := NewIterableSeries(func(*Serie) {}, 1)
	iterableSeries.Append(&Serie{Name: "serie1"})

	// Next call to Append must not block
	go iterableSeries.IterationStopped()
	iterableSeries.Append(&Serie{Name: "serie2"})
	iterableSeries.Append(&Serie{Name: "serie3"})
}

func TestIterableStreamJSONMarshalerAdapter(t *testing.T) {
	var series Series
	series = append(series, &Serie{Name: "serie1"})
	series = append(series, &Serie{Name: "serie2"})
	series = append(series, &Serie{Name: "serie3"})

	iterableSeries := NewIterableSeries(func(*Serie) {}, 3)
	for _, serie := range series {
		iterableSeries.Append(serie)
	}
	iterableSeries.SenderStopped()

	adapter := marshaler.NewIterableStreamJSONMarshalerAdapter(series)
	expected := dumpIterableStream(adapter)
	assert.EqualValues(t, expected, dumpIterableStream(iterableSeries))
}

func dumpIterableStream(marshaler marshaler.IterableStreamJSONMarshaler) []byte {
	jsonStream := jsoniter.NewStream(jsoniter.ConfigDefault, nil, 0)
	defer marshaler.IterationStopped()
	marshaler.WriteHeader(jsonStream)
	for marshaler.MoveNext() {
		marshaler.WriteCurrentItem(jsonStream)
	}
	marshaler.WriteFooter(jsonStream)
	return jsonStream.Buffer()
}
