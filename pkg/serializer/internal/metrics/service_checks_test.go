// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && test

package metrics

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	"github.com/DataDog/datadog-agent/pkg/config/mock"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
)

func createServiceCheck(checkName string) *servicecheck.ServiceCheck {
	return &servicecheck.ServiceCheck{
		CheckName:  checkName,
		Host:       "2",
		Ts:         3,
		Status:     servicecheck.ServiceCheckUnknown,
		Message:    "4",
		Tags:       []string{"5", "6"},
		OriginInfo: taggertypes.OriginInfo{},
	}
}

func buildPayload(t *testing.T, m marshaler.StreamJSONMarshaler, cfg pkgconfigmodel.Config) [][]byte {
	compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: cfg}).Comp
	builder := stream.NewJSONPayloadBuilder(true, cfg, compressor, logmock.New(t))
	payloads, err := stream.BuildJSONPayload(builder, m)
	assert.NoError(t, err)
	var uncompressedPayloads [][]byte

	for _, compressedPayload := range payloads {
		payload, err := compressor.Decompress(compressedPayload.GetContent())
		assert.NoError(t, err)

		uncompressedPayloads = append(uncompressedPayloads, payload)
	}
	return uncompressedPayloads
}

func assertEqualTo(t *testing.T, m marshaler.StreamJSONMarshaler, expect string) {
	config := mock.New(t)
	payloads := buildPayload(t, m, config)
	assert.Equal(t, 1, len(payloads))
	assert.Equal(t, expect, string(payloads[0]))
}

func TestServiceCheckDescribeItem(t *testing.T) {
	serviceChecks := ServiceChecks{createServiceCheck("check")}
	assert.Equal(t, `CheckName:"check", Message:"4"`, serviceChecks.DescribeItem(0))
}

func TestPayloadsNoServiceCheck(t *testing.T) {
	serviceChecks := ServiceChecks{}
	assertEqualTo(t, serviceChecks, "[]")
}

func TestPayloadsSingleServiceCheck(t *testing.T) {
	serviceChecks := ServiceChecks{createServiceCheck("checkName")}
	assertEqualTo(t, serviceChecks, "[{\"check\":\"checkName\",\"host_name\":\"2\",\"timestamp\":3,\"status\":3,\"message\":\"4\",\"tags\":[\"5\",\"6\"]}]")
}

func TestPayloadsEmptyServiceCheck(t *testing.T) {
	serviceChecks := ServiceChecks{&servicecheck.ServiceCheck{}}
	assertEqualTo(t, serviceChecks, "[{\"check\":\"\",\"host_name\":\"\",\"timestamp\":0,\"status\":0,\"message\":\"\",\"tags\":null}]")
}

func TestPayloadsServiceChecks(t *testing.T) {
	config := mock.New(t)
	config.Set("serializer_max_payload_size", 250, pkgconfigmodel.SourceAgentRuntime)

	serviceChecks := ServiceChecks{
		createServiceCheck("1"), createServiceCheck("2"), createServiceCheck("3"),
		createServiceCheck("4"), createServiceCheck("5"), createServiceCheck("6"),
		createServiceCheck("7"), createServiceCheck("8"),
	}

	payloads := buildPayload(t, serviceChecks, config)
	assert.Equal(t, 3, len(payloads))

	assert.Equal(t, "["+
		"{\"check\":\"1\",\"host_name\":\"2\",\"timestamp\":3,\"status\":3,\"message\":\"4\",\"tags\":[\"5\",\"6\"]},"+
		"{\"check\":\"2\",\"host_name\":\"2\",\"timestamp\":3,\"status\":3,\"message\":\"4\",\"tags\":[\"5\",\"6\"]},"+
		"{\"check\":\"3\",\"host_name\":\"2\",\"timestamp\":3,\"status\":3,\"message\":\"4\",\"tags\":[\"5\",\"6\"]}]",
		string(payloads[0]))

	assert.Equal(t, "["+
		"{\"check\":\"4\",\"host_name\":\"2\",\"timestamp\":3,\"status\":3,\"message\":\"4\",\"tags\":[\"5\",\"6\"]},"+
		"{\"check\":\"5\",\"host_name\":\"2\",\"timestamp\":3,\"status\":3,\"message\":\"4\",\"tags\":[\"5\",\"6\"]},"+
		"{\"check\":\"6\",\"host_name\":\"2\",\"timestamp\":3,\"status\":3,\"message\":\"4\",\"tags\":[\"5\",\"6\"]}]",
		string(payloads[1]))

	assert.Equal(t, "["+
		"{\"check\":\"7\",\"host_name\":\"2\",\"timestamp\":3,\"status\":3,\"message\":\"4\",\"tags\":[\"5\",\"6\"]},"+
		"{\"check\":\"8\",\"host_name\":\"2\",\"timestamp\":3,\"status\":3,\"message\":\"4\",\"tags\":[\"5\",\"6\"]}]",
		string(payloads[2]))
}

func createServiceChecks(numberOfItem int) ServiceChecks {
	var serviceCheckCollections []*servicecheck.ServiceCheck

	for i := 0; i < numberOfItem; i++ {
		serviceCheckCollections = append(serviceCheckCollections, createServiceCheck(fmt.Sprint(i)))
	}
	return ServiceChecks(serviceCheckCollections)
}

func benchmarkJSONPayloadBuilderServiceCheck(b *testing.B, numberOfItem int) {
	mockConfig := mock.New(b)
	compressor := metricscompression.NewCompressorReq(metricscompression.Requires{Cfg: mockConfig}).Comp
	payloadBuilder := stream.NewJSONPayloadBuilder(true, mockConfig, compressor, logmock.New(b))
	serviceChecks := createServiceChecks(numberOfItem)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		stream.BuildJSONPayload(payloadBuilder, serviceChecks)
	}
}

func BenchmarkJSONPayloadBuilderServiceCheck1(b *testing.B) {
	benchmarkJSONPayloadBuilderServiceCheck(b, 1)
}
func BenchmarkJSONPayloadBuilderServiceCheck10(b *testing.B) {
	benchmarkJSONPayloadBuilderServiceCheck(b, 10)
}
func BenchmarkJSONPayloadBuilderServiceCheck100(b *testing.B) {
	benchmarkJSONPayloadBuilderServiceCheck(b, 100)
}
func BenchmarkJSONPayloadBuilderServiceCheck1000(b *testing.B) {
	benchmarkJSONPayloadBuilderServiceCheck(b, 1000)
}
func BenchmarkJSONPayloadBuilderServiceCheck10000(b *testing.B) {
	benchmarkJSONPayloadBuilderServiceCheck(b, 10000)
}
func BenchmarkJSONPayloadBuilderServiceCheck100000(b *testing.B) {
	benchmarkJSONPayloadBuilderServiceCheck(b, 100000)
}
func BenchmarkJSONPayloadBuilderServiceCheck1000000(b *testing.B) {
	benchmarkJSONPayloadBuilderServiceCheck(b, 1000000)
}
func BenchmarkJSONPayloadBuilderServiceCheck10000000(b *testing.B) {
	benchmarkJSONPayloadBuilderServiceCheck(b, 10000000)
}
