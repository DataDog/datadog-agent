// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package metrics

import (
	"strings"
	"testing"

	"github.com/richardartoul/molecule"
	"github.com/richardartoul/molecule/src/codec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	noopimpl "github.com/DataDog/datadog-agent/pkg/util/compression/impl-noop"
)

// decodeSerieResourceTypes decodes a MetricPayload protobuf buffer (as produced by
// PayloadsBuilder.writeSerie) and returns, for each Series entry, the ordered list of
// its resource "type" strings (field 1 of MetricPayload, each entry field 1 of Series).
func decodeSerieResourceTypes(t *testing.T, buf []byte) [][]string {
	t.Helper()

	const payloadSeries = 1
	const seriesResources = 1
	const resourceType = 1

	var result [][]string
	codecBuf := codec.NewBuffer(buf)
	err := molecule.MessageEach(codecBuf, func(fieldNum int32, value molecule.Value) (bool, error) {
		if fieldNum != payloadSeries {
			return true, nil
		}

		serieBuf := codec.NewBuffer(value.Bytes)
		var types []string
		innerErr := molecule.MessageEach(serieBuf, func(innerFieldNum int32, innerValue molecule.Value) (bool, error) {
			if innerFieldNum != seriesResources {
				return true, nil
			}

			resourceBuf := codec.NewBuffer(innerValue.Bytes)
			return true, molecule.MessageEach(resourceBuf, func(resourceFieldNum int32, resourceValue molecule.Value) (bool, error) {
				if resourceFieldNum == resourceType {
					types = append(types, string(resourceValue.Bytes))
				}
				return true, nil
			})
		})
		require.NoError(t, innerErr)

		result = append(result, types)
		return true, nil
	})
	require.NoError(t, err)

	return result
}

func TestPayloadsBuilderHostlessSerieHasNoHostResource(t *testing.T) {
	r := require.New(t)

	bufferContext := marshaler.NewBufferContext()
	cfg := configmock.New(t)
	pipelineContext := &PipelineContext{}

	series := IterableSeries{}
	builder, err := series.NewPayloadsBuilder(
		bufferContext,
		cfg,
		noopimpl.New(),
		PipelineConfig{Filter: AllowAllFilter{}},
		pipelineContext,
	)
	r.NoError(err)

	r.NoError(builder.startPayload())

	r.NoError(builder.writeSerie(&metrics.Serie{
		Name:   "with.host",
		Host:   "example.com",
		Points: []metrics.Point{{Ts: 1, Value: 1}},
	}))
	r.NoError(builder.writeSerie(&metrics.Serie{
		Name:   "without.host",
		Points: []metrics.Point{{Ts: 1, Value: 1}},
	}))

	r.NoError(builder.finishPayload())

	r.Len(pipelineContext.payloads, 1)
	serieResourceTypes := decodeSerieResourceTypes(t, pipelineContext.payloads[0].GetContent())
	r.Len(serieResourceTypes, 2)

	assert.Contains(t, serieResourceTypes[0], "host")
	assert.NotContains(t, serieResourceTypes[1], "host")
}

func TestIterableSeriesMoveNext(t *testing.T) {
	r := require.New(t)
	series := metrics.Series{
		&metrics.Serie{Name: "serie1", NoIndex: true},
		&metrics.Serie{Name: "serie2", NoIndex: false},
		&metrics.Serie{Name: "serie3", NoIndex: false},
		&metrics.Serie{Name: "serie4", NoIndex: true},
	}
	iterableSerie := CreateIterableSeries(CreateSerieSource(series))
	r.True(iterableSerie.MoveNext()) // Skip serie1
	r.True(strings.Contains(iterableSerie.DescribeCurrentItem(), "serie2"))
	r.True(iterableSerie.MoveNext())
	r.True(strings.Contains(iterableSerie.DescribeCurrentItem(), "serie3"))
	r.False(iterableSerie.MoveNext()) // Skip serie4
}
