// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package serializer

import (
	"fmt"
	"net/http"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
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
	jsonPayloads     = forwarder.Payloads{}
	protobufPayloads = forwarder.Payloads{}
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
func (p *testPayload) SplitPayload(int) ([]marshaler.Marshaler, error) {
	return []marshaler.Marshaler{}, nil
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
func (p *testErrorPayload) SplitPayload(int) ([]marshaler.Marshaler, error) {
	return []marshaler.Marshaler{}, fmt.Errorf("some error")
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

func mkPayloads(payload []byte, compress bool) (forwarder.Payloads, error) {
	payloads := forwarder.Payloads{}
	var err error
	if compress {
		payload, err = compression.Compress(nil, payload)
		if err != nil {
			return nil, err
		}
	}
	payloads = append(payloads, &payload)
	return payloads, nil
}

type testEventsPayload struct {
	marshaler.Marshaler
	mock.Mock
}

func createTestEventsPayloadMock(marshaler marshaler.StreamJSONMarshaler) *testEventsPayload {
	p := &testEventsPayload{}
	p.Marshaler = marshaler
	return p
}

func createTestEventsPayload(marshaler marshaler.StreamJSONMarshaler) *testEventsPayload {
	p := createTestEventsPayloadMock(marshaler)
	p.On("CreateSingleMarshaler").Return(marshaler)
	return p
}

func (t *testEventsPayload) CreateSingleMarshaler() marshaler.StreamJSONMarshaler {
	args := t.Called()
	return args.Get(0).(marshaler.StreamJSONMarshaler)
}

func (t *testEventsPayload) CreateMarshalersBySourceType() []marshaler.StreamJSONMarshaler {
	args := t.Called()
	return args.Get(0).([]marshaler.StreamJSONMarshaler)
}

func TestSendV1Events(t *testing.T) {
	config.Datadog.Set("enable_events_stream_payload_serialization", false)
	defer config.Datadog.Set("enable_events_stream_payload_serialization", nil)

	f := &forwarder.MockedForwarder{}
	f.On("SubmitV1Intake", jsonPayloads, jsonExtraHeadersWithCompression).Return(nil).Times(1)

	s := NewSerializer(f)

	payload := createTestEventsPayload(&testPayload{})
	err := s.SendEvents(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := createTestEventsPayload(&testErrorPayload{})
	err = s.SendEvents(errPayload)
	require.NotNil(t, err)
}

type testPayloadMutipleValues struct {
	testPayload
	count int
}

func (p *testPayloadMutipleValues) Len() int { return p.count }

func TestSendV1EventsCreateMarshalersBySourceType(t *testing.T) {
	config.Datadog.Set("enable_events_stream_payload_serialization", true)
	defer config.Datadog.Set("enable_events_stream_payload_serialization", nil)
	f := &forwarder.MockedForwarder{}
	f.On("SubmitV1Intake", mock.Anything, jsonExtraHeadersWithCompression).Return(nil)
	s := NewSerializer(f)

	payload := &testPayloadMutipleValues{count: 1}

	eventPayload := createTestEventsPayloadMock(payload)
	eventPayload.On("CreateSingleMarshaler").Return(payload)
	err := s.SendEvents(eventPayload)
	assert.NoError(t, err)
	eventPayload.AssertExpectations(t)

	config.Datadog.Set("serializer_max_payload_size", 0)
	defer config.Datadog.Set("serializer_max_payload_size", nil)
	eventPayload.On("CreateMarshalersBySourceType").Return([]marshaler.StreamJSONMarshaler{payload})
	err = s.SendEvents(eventPayload)
	assert.NoError(t, err)
	eventPayload.AssertNumberOfCalls(t, "CreateMarshalersBySourceType", 1)

	payload.count = maxItemCountForCreateMarshalersBySourceType + 1
	eventPayload.On("CreateMarshalersBySourceType").Return([]marshaler.StreamJSONMarshaler{payload})
	err = s.SendEvents(eventPayload)
	assert.NoError(t, err)
	// CreateMarshalersBySourceType should not be called
	eventPayload.AssertNumberOfCalls(t, "CreateMarshalersBySourceType", 1)
}

func TestSendEvents(t *testing.T) {
	mockConfig := config.Mock()

	f := &forwarder.MockedForwarder{}
	f.On("SubmitEvents", protobufPayloads, protobufExtraHeadersWithCompression).Return(nil).Times(1)
	mockConfig.Set("use_v2_api.events", true)
	defer mockConfig.Set("use_v2_api.events", nil)

	s := NewSerializer(f)

	payload := createTestEventsPayload(&testPayload{})
	err := s.SendEvents(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := createTestEventsPayload(&testErrorPayload{})
	err = s.SendEvents(errPayload)
	require.NotNil(t, err)
}

func TestSendV1ServiceChecks(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	f.On("SubmitV1CheckRuns", jsonPayloads, jsonExtraHeadersWithCompression).Return(nil).Times(1)
	config.Datadog.Set("enable_service_checks_stream_payload_serialization", false)
	defer config.Datadog.Set("enable_service_checks_stream_payload_serialization", nil)

	s := NewSerializer(f)
	payload := &testPayload{}
	err := s.SendServiceChecks(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendServiceChecks(errPayload)
	require.NotNil(t, err)
}

func TestSendServiceChecks(t *testing.T) {
	mockConfig := config.Mock()

	f := &forwarder.MockedForwarder{}
	f.On("SubmitServiceChecks", protobufPayloads, protobufExtraHeadersWithCompression).Return(nil).Times(1)
	mockConfig.Set("use_v2_api.service_checks", true)
	defer mockConfig.Set("use_v2_api.service_checks", nil)

	s := NewSerializer(f)

	payload := &testPayload{}
	err := s.SendServiceChecks(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendServiceChecks(errPayload)
	require.NotNil(t, err)
}

func TestSendV1Series(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	f.On("SubmitV1Series", jsonPayloads, jsonExtraHeadersWithCompression).Return(nil).Times(1)
	config.Datadog.Set("enable_stream_payload_serialization", false)
	defer config.Datadog.Set("enable_stream_payload_serialization", nil)

	s := NewSerializer(f)

	payload := &testPayload{}
	err := s.SendSeries(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendSeries(errPayload)
	require.NotNil(t, err)
}

func TestSendSeries(t *testing.T) {
	mockConfig := config.Mock()

	f := &forwarder.MockedForwarder{}
	f.On("SubmitSeries", protobufPayloads, protobufExtraHeadersWithCompression).Return(nil).Times(1)
	mockConfig.Set("use_v2_api.series", true)
	defer mockConfig.Set("use_v2_api.series", nil)

	s := NewSerializer(f)

	payload := &testPayload{}
	err := s.SendSeries(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendSeries(errPayload)
	require.NotNil(t, err)
}

func TestSendSketch(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	payloads, _ := mkPayloads(protobufString, true)
	f.On("SubmitSketchSeries", payloads, protobufExtraHeadersWithCompression).Return(nil).Times(1)

	s := NewSerializer(f)

	payload := &testPayload{}
	err := s.SendSketch(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendSketch(errPayload)
	require.NotNil(t, err)
}

func TestSendMetadata(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	f.On("SubmitMetadata", jsonPayloads, jsonExtraHeadersWithCompression).Return(nil).Times(1)

	s := NewSerializer(f)

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

func TestSendJSONToV1Intake(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	payload := []byte("\"test\"")
	payloads, _ := mkPayloads(payload, false)
	f.On("SubmitV1Intake", payloads, jsonExtraHeaders).Return(nil).Times(1)

	s := NewSerializer(f)

	err := s.SendJSONToV1Intake("test")
	require.Nil(t, err)
	f.AssertExpectations(t)

	f.On("SubmitV1Intake", payloads, jsonExtraHeaders).Return(fmt.Errorf("some error")).Times(1)
	err = s.SendJSONToV1Intake("test")
	require.NotNil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendJSONToV1Intake(errPayload)
	require.NotNil(t, err)
}

func TestSendWithDisabledKind(t *testing.T) {
	mockConfig := config.Mock()

	mockConfig.Set("enable_payloads.events", false)
	mockConfig.Set("enable_payloads.series", false)
	mockConfig.Set("enable_payloads.service_checks", false)
	mockConfig.Set("enable_payloads.sketches", false)
	mockConfig.Set("enable_payloads.json_to_v1_intake", false)

	//restore default values
	defer func() {
		mockConfig.Set("enable_payloads.events", true)
		mockConfig.Set("enable_payloads.series", true)
		mockConfig.Set("enable_payloads.service_checks", true)
		mockConfig.Set("enable_payloads.sketches", true)
		mockConfig.Set("enable_payloads.json_to_v1_intake", true)
	}()

	f := &forwarder.MockedForwarder{}
	s := NewSerializer(f)

	payload := &testPayload{}
	payloadEvents := createTestEventsPayload(payload)

	s.SendEvents(payloadEvents)
	s.SendSeries(payload)
	s.SendSketch(payload)
	s.SendServiceChecks(payload)
	s.SendJSONToV1Intake("test")

	f.AssertNotCalled(t, "SubmitMetadata")
	f.AssertNotCalled(t, "SubmitEvents")
	f.AssertNotCalled(t, "SubmitV1CheckRuns")
	f.AssertNotCalled(t, "SubmitServiceChecks")
	f.AssertNotCalled(t, "SubmitV1Series")
	f.AssertNotCalled(t, "SubmitSeries")
	f.AssertNotCalled(t, "SubmitSketchSeries")

	// We never disable metadata
	f.On("SubmitMetadata", jsonPayloads, jsonExtraHeadersWithCompression).Return(nil).Times(1)
	s.SendMetadata(payload)
	f.AssertNumberOfCalls(t, "SubmitMetadata", 1) // called once for the metadata
}
