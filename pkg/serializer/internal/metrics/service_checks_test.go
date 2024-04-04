// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build zlib && test

package metrics

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	"github.com/DataDog/datadog-agent/pkg/serializer/internal/stream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/split"
	taggertypes "github.com/DataDog/datadog-agent/pkg/tagger/types"
)

func TestMarshalJSONServiceChecks(t *testing.T) {
	serviceChecks := ServiceChecks{{
		CheckName:  "my_service.can_connect",
		Host:       "my-hostname",
		Ts:         int64(12345),
		Status:     servicecheck.ServiceCheckOK,
		Message:    "my_service is up",
		Tags:       []string{"tag1", "tag2:yes"},
		OriginInfo: taggertypes.OriginInfo{},
	}}

	payload, err := serviceChecks.MarshalJSON()
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, payload, []byte("[{\"check\":\"my_service.can_connect\",\"host_name\":\"my-hostname\",\"timestamp\":12345,\"status\":0,\"message\":\"my_service is up\",\"tags\":[\"tag1\",\"tag2:yes\"]}]\n"))
}

func TestSplitServiceChecks(t *testing.T) {
	var serviceChecks = ServiceChecks{}
	for i := 0; i < 2; i++ {
		sc := servicecheck.ServiceCheck{
			CheckName:  "test.check",
			Host:       "test.localhost",
			Ts:         1000,
			Status:     servicecheck.ServiceCheckOK,
			Message:    "this is fine",
			Tags:       []string{"tag1", "tag2:yes"},
			OriginInfo: taggertypes.OriginInfo{},
		}
		serviceChecks = append(serviceChecks, &sc)
	}

	newSC, err := serviceChecks.SplitPayload(2)
	require.Nil(t, err)
	require.Len(t, newSC, 2)
	require.Equal(t, 2, len(newSC))

	newSC, err = serviceChecks.SplitPayload(3)
	require.Nil(t, err)
	require.Len(t, newSC, 2)
}

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
	strategy := compressionimpl.NewCompressor(cfg)
	builder := stream.NewJSONPayloadBuilder(true, cfg, strategy)
	payloads, err := stream.BuildJSONPayload(builder, m)
	assert.NoError(t, err)
	var uncompressedPayloads [][]byte

	for _, compressedPayload := range payloads {
		payload, err := strategy.Decompress(compressedPayload.GetContent())
		assert.NoError(t, err)

		uncompressedPayloads = append(uncompressedPayloads, payload)
	}
	return uncompressedPayloads
}

func assertEqualToMarshalJSON(t *testing.T, m marshaler.StreamJSONMarshaler, jsonMarshaler marshaler.JSONMarshaler) {
	config := pkgconfigsetup.Conf()
	payloads := buildPayload(t, m, config)
	json, err := jsonMarshaler.MarshalJSON()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(payloads))
	assert.Equal(t, strings.TrimSpace(string(json)), string(payloads[0]))
}

func TestServiceCheckDescribeItem(t *testing.T) {
	serviceChecks := ServiceChecks{createServiceCheck("check")}
	assert.Equal(t, `CheckName:"check", Message:"4"`, serviceChecks.DescribeItem(0))
}

func TestPayloadsNoServiceCheck(t *testing.T) {
	serviceChecks := ServiceChecks{}
	assertEqualToMarshalJSON(t, serviceChecks, serviceChecks)
}

func TestPayloadsSingleServiceCheck(t *testing.T) {
	serviceChecks := ServiceChecks{createServiceCheck("checkName")}
	assertEqualToMarshalJSON(t, serviceChecks, serviceChecks)
}

func TestPayloadsEmptyServiceCheck(t *testing.T) {
	serviceChecks := ServiceChecks{&servicecheck.ServiceCheck{}}
	assertEqualToMarshalJSON(t, serviceChecks, serviceChecks)
}

func TestPayloadsServiceChecks(t *testing.T) {
	config := pkgconfigsetup.Conf()
	config.Set("serializer_max_payload_size", 200, pkgconfigmodel.SourceAgentRuntime)

	serviceCheckCollection := []ServiceChecks{
		{createServiceCheck("1"), createServiceCheck("2"), createServiceCheck("3")},
		{createServiceCheck("4"), createServiceCheck("5"), createServiceCheck("6")},
		{createServiceCheck("7"), createServiceCheck("8")}}
	var allServiceChecks ServiceChecks
	for _, serviceCheck := range serviceCheckCollection {
		allServiceChecks = append(allServiceChecks, serviceCheck...)
	}

	payloads := buildPayload(t, allServiceChecks, config)
	assert.Equal(t, 3, len(payloads))

	for index, serviceChecks := range serviceCheckCollection {
		json, err := serviceChecks.MarshalJSON()
		assert.NoError(t, err)

		assert.Equal(t, strings.TrimSpace(string(json)), string(payloads[index]))
	}
}

func createServiceChecks(numberOfItem int) ServiceChecks {
	var serviceCheckCollections []*servicecheck.ServiceCheck

	for i := 0; i < numberOfItem; i++ {
		serviceCheckCollections = append(serviceCheckCollections, createServiceCheck(fmt.Sprint(i)))
	}
	return ServiceChecks(serviceCheckCollections)
}

func benchmarkJSONPayloadBuilderServiceCheck(b *testing.B, numberOfItem int) {
	payloadBuilder := stream.NewJSONPayloadBuilder(true, pkgconfigsetup.Conf(), compressionimpl.NewCompressor(pkgconfigsetup.Conf()))
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

func benchmarkPayloadsServiceCheck(b *testing.B, numberOfItem int) {
	serviceChecks := createServiceChecks(numberOfItem)

	b.ResetTimer()

	mockConfig := pkgconfigsetup.Conf()
	strategy := compressionimpl.NewCompressor(mockConfig)
	for n := 0; n < b.N; n++ {
		split.Payloads(serviceChecks, true, split.JSONMarshalFct, strategy)
	}
}

func BenchmarkPayloadServiceCheck1(b *testing.B)      { benchmarkPayloadsServiceCheck(b, 1) }
func BenchmarkPayloadServiceCheck10(b *testing.B)     { benchmarkPayloadsServiceCheck(b, 10) }
func BenchmarkPayloadServiceCheck100(b *testing.B)    { benchmarkPayloadsServiceCheck(b, 100) }
func BenchmarkPayloadServiceCheck1000(b *testing.B)   { benchmarkPayloadsServiceCheck(b, 1000) }
func BenchmarkPayloadServiceCheck10000(b *testing.B)  { benchmarkPayloadsServiceCheck(b, 10000) }
func BenchmarkPayloadServiceCheck100000(b *testing.B) { benchmarkPayloadsServiceCheck(b, 100000) }
func BenchmarkPayloadServiceCheck1000000(b *testing.B) {
	benchmarkPayloadsServiceCheck(b, 1000000)
}
func BenchmarkPayloadServiceCheck10000000(b *testing.B) {
	benchmarkPayloadsServiceCheck(b, 10000000)
}
