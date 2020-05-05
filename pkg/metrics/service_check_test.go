// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

//+build zlib

package metrics

import (
	"strings"
	"testing"

	"github.com/gogo/protobuf/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentpayload "github.com/DataDog/agent-payload/gogen"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/serializer/jsonstream"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/serializer/split"
)

func TestMarshalServiceChecks(t *testing.T) {
	serviceChecks := ServiceChecks{{
		CheckName: "test.check",
		Host:      "test.localhost",
		Ts:        1000,
		Status:    ServiceCheckOK,
		Message:   "this is fine",
		Tags:      []string{"tag1", "tag2:yes"},
	}}

	payload, err := serviceChecks.Marshal()
	assert.Nil(t, err)
	assert.NotNil(t, payload)

	newPayload := &agentpayload.ServiceChecksPayload{}
	err = proto.Unmarshal(payload, newPayload)
	assert.Nil(t, err)

	require.Len(t, newPayload.ServiceChecks, 1)
	assert.Equal(t, newPayload.ServiceChecks[0].Name, "test.check")
	assert.Equal(t, newPayload.ServiceChecks[0].Host, "test.localhost")
	assert.Equal(t, newPayload.ServiceChecks[0].Ts, int64(1000))
	assert.Equal(t, newPayload.ServiceChecks[0].Status, int32(ServiceCheckOK))
	assert.Equal(t, newPayload.ServiceChecks[0].Message, "this is fine")
	require.Len(t, newPayload.ServiceChecks[0].Tags, 2)
	assert.Equal(t, newPayload.ServiceChecks[0].Tags[0], "tag1")
	assert.Equal(t, newPayload.ServiceChecks[0].Tags[1], "tag2:yes")
}

func TestMarshalJSONServiceChecks(t *testing.T) {
	serviceChecks := ServiceChecks{{
		CheckName: "my_service.can_connect",
		Host:      "my-hostname",
		Ts:        int64(12345),
		Status:    ServiceCheckOK,
		Message:   "my_service is up",
		Tags:      []string{"tag1", "tag2:yes"},
	}}

	payload, err := serviceChecks.MarshalJSON()
	assert.Nil(t, err)
	assert.NotNil(t, payload)
	assert.Equal(t, payload, []byte("[{\"check\":\"my_service.can_connect\",\"host_name\":\"my-hostname\",\"timestamp\":12345,\"status\":0,\"message\":\"my_service is up\",\"tags\":[\"tag1\",\"tag2:yes\"]}]\n"))
}

func TestSplitServiceChecks(t *testing.T) {
	var serviceChecks = ServiceChecks{}
	for i := 0; i < 2; i++ {
		sc := ServiceCheck{
			CheckName: "test.check",
			Host:      "test.localhost",
			Ts:        1000,
			Status:    ServiceCheckOK,
			Message:   "this is fine",
			Tags:      []string{"tag1", "tag2:yes"},
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

func createServiceCheck(checkName string) *ServiceCheck {
	return &ServiceCheck{
		CheckName: checkName,
		Host:      "2",
		Ts:        3,
		Status:    ServiceCheckUnknown,
		Message:   "4",
		Tags:      []string{"5", "6"}}
}

func buildPayload(t *testing.T, m marshaler.StreamJSONMarshaler) [][]byte {
	builder := jsonstream.NewPayloadBuilder()
	payloads, err := builder.Build(m)
	assert.NoError(t, err)
	var uncompressedPayloads [][]byte

	for _, compressedPayload := range payloads {
		payload, err := decompressPayload(*compressedPayload)
		assert.NoError(t, err)

		uncompressedPayloads = append(uncompressedPayloads, payload)
	}
	return uncompressedPayloads
}

func assertEqualToMarshalJSON(t *testing.T, m marshaler.StreamJSONMarshaler) {
	payloads := buildPayload(t, m)
	json, err := m.MarshalJSON()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(payloads))
	assert.Equal(t, strings.TrimSpace(string(json)), string(payloads[0]))
}

func TestServiceCheckDescribeItem(t *testing.T) {
	serviceChecks := ServiceChecks{createServiceCheck("check")}
	assert.Equal(t, `CheckName:"check", Message:"4"`, serviceChecks.DescribeItem(0))
}

func TestPayloadsNoServiceCheck(t *testing.T) {
	assertEqualToMarshalJSON(t, ServiceChecks{})
}

func TestPayloadsSingleServiceCheck(t *testing.T) {
	serviceChecks := ServiceChecks{createServiceCheck("checkName")}
	assertEqualToMarshalJSON(t, serviceChecks)
}

func TestPayloadsEmptyServiceCheck(t *testing.T) {
	assertEqualToMarshalJSON(t, ServiceChecks{&ServiceCheck{}})
}

func TestPayloadsServiceChecks(t *testing.T) {

	maxPayloadSize := config.Datadog.GetInt("serializer_max_payload_size")
	config.Datadog.SetDefault("serializer_max_payload_size", 200)
	defer config.Datadog.SetDefault("serializer_max_payload_size", maxPayloadSize)

	serviceCheckCollection := []ServiceChecks{
		{createServiceCheck("1"), createServiceCheck("2"), createServiceCheck("3")},
		{createServiceCheck("4"), createServiceCheck("5"), createServiceCheck("6")},
		{createServiceCheck("7"), createServiceCheck("8")}}
	var allServiceChecks ServiceChecks
	for _, serviceCheck := range serviceCheckCollection {
		allServiceChecks = append(allServiceChecks, serviceCheck...)
	}

	payloads := buildPayload(t, allServiceChecks)
	assert.Equal(t, 3, len(payloads))

	for index, serviceChecks := range serviceCheckCollection {
		json, err := serviceChecks.MarshalJSON()
		assert.NoError(t, err)

		assert.Equal(t, strings.TrimSpace(string(json)), string(payloads[index]))
	}
}

func createServiceChecks(numberOfItem int) ServiceChecks {
	var serviceCheckCollections []*ServiceCheck

	for i := 0; i < numberOfItem; i++ {
		serviceCheckCollections = append(serviceCheckCollections, createServiceCheck(string(i)))
	}
	return ServiceChecks(serviceCheckCollections)
}

func benchmarkPayloadBuilderServiceCheck(b *testing.B, numberOfItem int) {
	payloadBuilder := jsonstream.NewPayloadBuilder()
	serviceChecks := createServiceChecks(numberOfItem)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		payloadBuilder.Build(serviceChecks)
	}
}

func BenchmarkPayloadBuilderServiceCheck1(b *testing.B)  { benchmarkPayloadBuilderServiceCheck(b, 1) }
func BenchmarkPayloadBuilderServiceCheck10(b *testing.B) { benchmarkPayloadBuilderServiceCheck(b, 10) }
func BenchmarkPayloadBuilderServiceCheck100(b *testing.B) {
	benchmarkPayloadBuilderServiceCheck(b, 100)
}
func BenchmarkPayloadBuilderServiceCheck1000(b *testing.B) {
	benchmarkPayloadBuilderServiceCheck(b, 1000)
}
func BenchmarkPayloadBuilderServiceCheck10000(b *testing.B) {
	benchmarkPayloadBuilderServiceCheck(b, 10000)
}
func BenchmarkPayloadBuilderServiceCheck100000(b *testing.B) {
	benchmarkPayloadBuilderServiceCheck(b, 100000)
}
func BenchmarkPayloadBuilderServiceCheck1000000(b *testing.B) {
	benchmarkPayloadBuilderServiceCheck(b, 1000000)
}
func BenchmarkPayloadBuilderServiceCheck10000000(b *testing.B) {
	benchmarkPayloadBuilderServiceCheck(b, 10000000)
}

func benchmarkPayloadsServiceCheck(b *testing.B, numberOfItem int) {
	serviceChecks := createServiceChecks(numberOfItem)

	b.ResetTimer()

	for n := 0; n < b.N; n++ {
		split.Payloads(serviceChecks, true, split.MarshalJSON)
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
