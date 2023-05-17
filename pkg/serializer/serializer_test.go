// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package serializer

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	metricsserializer "github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

var initialContentEncoding = compression.ContentEncoding

func resetContentEncoding() {
	compression.ContentEncoding = initialContentEncoding
	initExtraHeaders()
}

func TestInitExtraHeadersNoopCompression(t *testing.T) {
	compression.ContentEncoding = ""
	defer resetContentEncoding()

	initExtraHeaders()

	expected := make(http.Header)
	expected.Set("Content-Type", jsonContentType)
	assert.Equal(t, expected, jsonExtraHeaders)

	expected = make(http.Header)
	expected.Set(payloadVersionHTTPHeader, AgentPayloadVersion)
	expected.Set("Content-Type", protobufContentType)
	assert.Equal(t, expected, protobufExtraHeaders)

	// No "Content-Encoding" header
	expected = make(http.Header)
	expected.Set("Content-Type", jsonContentType)
	assert.Equal(t, expected, jsonExtraHeadersWithCompression)

	expected = make(http.Header)
	expected.Set("Content-Type", protobufContentType)
	expected.Set(payloadVersionHTTPHeader, AgentPayloadVersion)
	assert.Equal(t, expected, protobufExtraHeadersWithCompression)
}

func TestInitExtraHeadersWithCompression(t *testing.T) {
	compression.ContentEncoding = "zstd"
	defer resetContentEncoding()

	initExtraHeaders()

	expected := make(http.Header)
	expected.Set("Content-Type", jsonContentType)
	assert.Equal(t, expected, jsonExtraHeaders)

	expected = make(http.Header)
	expected.Set("Content-Type", protobufContentType)
	expected.Set(payloadVersionHTTPHeader, AgentPayloadVersion)
	assert.Equal(t, expected, protobufExtraHeaders)

	// "Content-Encoding" header present with correct value
	expected = make(http.Header)
	expected.Set("Content-Type", jsonContentType)
	expected.Set("Content-Encoding", compression.ContentEncoding)
	assert.Equal(t, expected, jsonExtraHeadersWithCompression)

	expected = make(http.Header)
	expected.Set("Content-Type", protobufContentType)
	expected.Set("Content-Encoding", compression.ContentEncoding)
	expected.Set(payloadVersionHTTPHeader, AgentPayloadVersion)
	assert.Equal(t, expected, protobufExtraHeadersWithCompression)
}

func TestAgentPayloadVersion(t *testing.T) {
	assert.NotEmpty(t, AgentPayloadVersion, "AgentPayloadVersion is empty, indicates that the package was not built correctly")
}

var (
	jsonPayloads     = transaction.BytesPayloads{}
	protobufPayloads = transaction.BytesPayloads{}
	jsonHeader       = []byte("{")
	jsonFooter       = []byte("}")
	jsonItem         = []byte("TO JSON")
	jsonString       = []byte("{TO JSON}")
	protobufString   = []byte("TO PROTOBUF")
)

func init() {
	jsonPayloads, _ = mkPayloads(jsonString, true)
	protobufPayloads, _ = mkPayloads(protobufString, true)
}

type testPayload struct{}

func (p *testPayload) MarshalJSON() ([]byte, error) { return jsonString, nil }
func (p *testPayload) Marshal() ([]byte, error)     { return protobufString, nil }
func (p *testPayload) MarshalSplitCompress(bufferContext *marshaler.BufferContext) (transaction.BytesPayloads, error) {
	payloads := transaction.BytesPayloads{}
	payload, err := compression.Compress(protobufString)
	if err != nil {
		return nil, err
	}
	payloads = append(payloads, transaction.NewBytesPayloadWithoutMetaData(payload))
	return payloads, nil
}

func (p *testPayload) SplitPayload(int) ([]marshaler.AbstractMarshaler, error) {
	return []marshaler.AbstractMarshaler{}, nil
}

func (p *testPayload) WriteHeader(stream *jsoniter.Stream) error {
	_, err := stream.Write(jsonHeader)
	return err
}

func (p *testPayload) WriteFooter(stream *jsoniter.Stream) error {
	_, err := stream.Write(jsonFooter)
	return err
}

func (p *testPayload) WriteItem(stream *jsoniter.Stream, i int) error {
	_, err := stream.Write(jsonItem)
	return err
}
func (p *testPayload) Len() int                  { return 1 }
func (p *testPayload) DescribeItem(i int) string { return "description" }

type testErrorPayload struct{}

func (p *testErrorPayload) MarshalJSON() ([]byte, error) { return nil, fmt.Errorf("some error") }
func (p *testErrorPayload) Marshal() ([]byte, error)     { return nil, fmt.Errorf("some error") }
func (p *testErrorPayload) SplitPayload(int) ([]marshaler.AbstractMarshaler, error) {
	return []marshaler.AbstractMarshaler{}, fmt.Errorf("some error")
}

func (p *testErrorPayload) WriteHeader(stream *jsoniter.Stream) error {
	_, err := stream.Write(jsonHeader)
	return err
}

func (p *testErrorPayload) WriteFooter(stream *jsoniter.Stream) error {
	_, err := stream.Write(jsonFooter)
	return err
}

func (p *testErrorPayload) WriteItem(stream *jsoniter.Stream, i int) error {
	return fmt.Errorf("some error")
}
func (p *testErrorPayload) Len() int                  { return 1 }
func (p *testErrorPayload) DescribeItem(i int) string { return "description" }

func mkPayloads(payload []byte, compress bool) (transaction.BytesPayloads, error) {
	payloads := transaction.BytesPayloads{}
	var err error
	if compress {
		payload, err = compression.Compress(payload)
		if err != nil {
			return nil, err
		}
	}
	payloads = append(payloads, transaction.NewBytesPayloadWithoutMetaData(payload))
	return payloads, nil
}

func createJSONPayloadMatcher(prefix string) interface{} {
	return mock.MatchedBy(func(payloads transaction.BytesPayloads) bool {
		return doPayloadsMatch(payloads, prefix)
	})
}

func doPayloadsMatch(payloads transaction.BytesPayloads, prefix string) bool {
	for _, compressedPayload := range payloads {
		if payload, err := compression.Decompress(compressedPayload.GetContent()); err != nil {
			return false
		} else {
			if strings.HasPrefix(string(payload), prefix) {
				return true
			}
		}
	}
	return false
}

func createJSONBytesPayloadMatcher(prefix string) interface{} {
	return mock.MatchedBy(func(bytesPayloads transaction.BytesPayloads) bool {
		return doPayloadsMatch(bytesPayloads, prefix)
	})
}

func createProtoPayloadMatcher(content []byte) interface{} {
	return mock.MatchedBy(func(payloads transaction.BytesPayloads) bool {
		for _, compressedPayload := range payloads {
			if payload, err := compression.Decompress(compressedPayload.GetContent()); err != nil {
				return false
			} else {
				if reflect.DeepEqual(content, payload) {
					return true
				}
			}
		}
		return false
	})
}

func TestSendV1Events(t *testing.T) {
	config.Datadog.Set("enable_events_stream_payload_serialization", false)
	defer config.Datadog.Set("enable_events_stream_payload_serialization", nil)

	f := &forwarder.MockedForwarder{}

	matcher := createJSONPayloadMatcher(`{"apiKey":"","events":{},"internalHostname"`)
	f.On("SubmitV1Intake", matcher, jsonExtraHeadersWithCompression).Return(nil).Times(1)

	s := NewSerializer(f, nil)
	err := s.SendEvents([]*metrics.Event{})
	require.Nil(t, err)
	f.AssertExpectations(t)
}

func TestSendV1EventsCreateMarshalersBySourceType(t *testing.T) {
	config.Datadog.Set("enable_events_stream_payload_serialization", true)
	defer config.Datadog.Set("enable_events_stream_payload_serialization", nil)
	f := &forwarder.MockedForwarder{}

	s := NewSerializer(f, nil)

	events := metrics.Events{&metrics.Event{SourceTypeName: "source1"}, &metrics.Event{SourceTypeName: "source2"}, &metrics.Event{SourceTypeName: "source3"}}
	payloadsCountMatcher := func(payloadCount int) interface{} {
		return mock.MatchedBy(func(payloads transaction.BytesPayloads) bool {
			return len(payloads) == payloadCount
		})
	}

	f.On("SubmitV1Intake", payloadsCountMatcher(1), jsonExtraHeadersWithCompression).Return(nil)
	err := s.SendEvents(events)
	assert.NoError(t, err)
	f.AssertExpectations(t)

	config.Datadog.Set("serializer_max_payload_size", 20)
	defer config.Datadog.Set("serializer_max_payload_size", nil)

	f.On("SubmitV1Intake", payloadsCountMatcher(3), jsonExtraHeadersWithCompression).Return(nil)
	err = s.SendEvents(events)
	assert.NoError(t, err)
	f.AssertExpectations(t)
}

func TestSendV1ServiceChecks(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	matcher := createJSONPayloadMatcher(`[{"check":"","host_name":"","timestamp":0,"status":0,"message":"","tags":null}]`)
	f.On("SubmitV1CheckRuns", matcher, jsonExtraHeadersWithCompression).Return(nil).Times(1)
	config.Datadog.Set("enable_service_checks_stream_payload_serialization", false)
	defer config.Datadog.Set("enable_service_checks_stream_payload_serialization", nil)

	s := NewSerializer(f, nil)
	err := s.SendServiceChecks(metrics.ServiceChecks{&metrics.ServiceCheck{}})
	require.Nil(t, err)
	f.AssertExpectations(t)
}

func TestSendV1Series(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	matcher := createJSONBytesPayloadMatcher(`{"series":[]}`)

	f.On("SubmitV1Series", matcher, jsonExtraHeadersWithCompression).Return(nil).Times(1)
	config.Datadog.Set("enable_stream_payload_serialization", false)
	defer config.Datadog.Set("enable_stream_payload_serialization", nil)
	config.Datadog.Set("use_v2_api.series", false)
	defer config.Datadog.Set("use_v2_api.series", true)

	s := NewSerializer(f, nil)

	err := s.SendIterableSeries(metricsserializer.CreateSerieSource(metrics.Series{}))
	require.Nil(t, err)
	f.AssertExpectations(t)
}

func TestSendSeries(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	matcher := createProtoPayloadMatcher([]byte{0xa, 0xa, 0xa, 0x6, 0xa, 0x4, 0x68, 0x6f, 0x73, 0x74, 0x28, 0x3})
	f.On("SubmitSeries", matcher, protobufExtraHeadersWithCompression).Return(nil).Times(1)
	config.Datadog.Set("use_v2_api.series", true) // default value, but just to be sure

	s := NewSerializer(f, nil)

	err := s.SendIterableSeries(metricsserializer.CreateSerieSource(metrics.Series{&metrics.Serie{}}))
	require.Nil(t, err)
	f.AssertExpectations(t)
}

func TestSendSketch(t *testing.T) {
	f := &forwarder.MockedForwarder{}

	matcher := createProtoPayloadMatcher([]byte{18, 0})
	f.On("SubmitSketchSeries", matcher, protobufExtraHeadersWithCompression).Return(nil).Times(1)

	s := NewSerializer(f, nil)
	err := s.SendSketch(metrics.NewSketchesSourceTest())
	require.Nil(t, err)
	f.AssertExpectations(t)
}

func TestSendMetadata(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	f.On("SubmitMetadata", jsonPayloads, jsonExtraHeadersWithCompression).Return(nil).Times(1)

	s := NewSerializer(f, nil)

	payload := &testPayload{}
	err := s.SendMetadata(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	f.On("SubmitMetadata", jsonPayloads, jsonExtraHeadersWithCompression).Return(fmt.Errorf("some error")).Times(1)
	err = s.SendMetadata(payload)
	require.NotNil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendMetadata(errPayload)
	require.NotNil(t, err)
}

func TestSendProcessesMetadata(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	payload := []byte("\"test\"")
	payloads, _ := mkPayloads(payload, true)
	f.On("SubmitV1Intake", payloads, jsonExtraHeadersWithCompression).Return(nil).Times(1)

	s := NewSerializer(f, nil)

	err := s.SendProcessesMetadata("test")
	require.Nil(t, err)
	f.AssertExpectations(t)

	f.On("SubmitV1Intake", payloads, jsonExtraHeadersWithCompression).Return(fmt.Errorf("some error")).Times(1)
	err = s.SendProcessesMetadata("test")
	require.NotNil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendProcessesMetadata(errPayload)
	require.NotNil(t, err)
}

func TestSendWithDisabledKind(t *testing.T) {
	mockConfig := config.Mock(t)

	mockConfig.Set("enable_payloads.events", false)
	mockConfig.Set("enable_payloads.series", false)
	mockConfig.Set("enable_payloads.service_checks", false)
	mockConfig.Set("enable_payloads.sketches", false)
	mockConfig.Set("enable_payloads.json_to_v1_intake", false)

	// restore default values
	defer func() {
		mockConfig.Set("enable_payloads.events", true)
		mockConfig.Set("enable_payloads.series", true)
		mockConfig.Set("enable_payloads.service_checks", true)
		mockConfig.Set("enable_payloads.sketches", true)
		mockConfig.Set("enable_payloads.json_to_v1_intake", true)
	}()

	f := &forwarder.MockedForwarder{}
	s := NewSerializer(f, nil)

	payload := &testPayload{}

	s.SendEvents(make(metrics.Events, 0))
	s.SendIterableSeries(metricsserializer.CreateSerieSource(metrics.Series{}))
	s.SendSketch(metrics.NewSketchesSourceTest())
	s.SendServiceChecks(make(metrics.ServiceChecks, 0))
	s.SendProcessesMetadata("test")

	f.AssertNotCalled(t, "SubmitMetadata")
	f.AssertNotCalled(t, "SubmitV1CheckRuns")
	f.AssertNotCalled(t, "SubmitV1Series")
	f.AssertNotCalled(t, "SubmitSketchSeries")

	// We never disable metadata
	f.On("SubmitMetadata", jsonPayloads, jsonExtraHeadersWithCompression).Return(nil).Times(1)
	s.SendMetadata(payload)
	f.AssertNumberOfCalls(t, "SubmitMetadata", 1) // called once for the metadata
}
