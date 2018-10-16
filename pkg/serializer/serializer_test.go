// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package serializer

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
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
	jsonString       = []byte("TO JSON")
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

type testErrorPayload struct{}

func (p *testErrorPayload) MarshalJSON() ([]byte, error) { return nil, fmt.Errorf("some error") }
func (p *testErrorPayload) Marshal() ([]byte, error)     { return nil, fmt.Errorf("some error") }
func (p *testErrorPayload) SplitPayload(int) ([]marshaler.Marshaler, error) {
	return []marshaler.Marshaler{}, fmt.Errorf("some error")
}

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

func TestSendV1Events(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	f.On("SubmitV1Intake", jsonPayloads, jsonExtraHeadersWithCompression).Return(nil).Times(1)

	s := NewSerializer(f)

	payload := &testPayload{}
	err := s.SendEvents(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendEvents(errPayload)
	require.NotNil(t, err)
}

func TestSendEvents(t *testing.T) {
	mockConfig := config.NewMock()

	f := &forwarder.MockedForwarder{}
	f.On("SubmitEvents", protobufPayloads, protobufExtraHeadersWithCompression).Return(nil).Times(1)
	mockConfig.Set("use_v2_api.events", true)
	defer mockConfig.Set("use_v2_api.events", nil)

	s := NewSerializer(f)

	payload := &testPayload{}
	err := s.SendEvents(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendEvents(errPayload)
	require.NotNil(t, err)
}

func TestSendV1ServiceChecks(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	f.On("SubmitV1CheckRuns", jsonPayloads, jsonExtraHeadersWithCompression).Return(nil).Times(1)

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
	mockConfig := config.NewMock()

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
	mockConfig := config.NewMock()

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
	payloads, _ := mkPayloads(protobufString, false)
	f.On("SubmitSketchSeries", payloads, protobufExtraHeaders).Return(nil).Times(1)

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
	payloads, _ := mkPayloads(jsonString, false)
	f.On("SubmitV1Intake", payloads, jsonExtraHeaders).Return(nil).Times(1)

	s := NewSerializer(f)

	payload := &testPayload{}
	err := s.SendMetadata(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	f.On("SubmitV1Intake", payloads, jsonExtraHeaders).Return(fmt.Errorf("some error")).Times(1)
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
	mockConfig := config.NewMock()

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
	payloads, _ := mkPayloads(jsonString, false)

	s.SendEvents(payload)
	s.SendSeries(payload)
	s.SendSketch(payload)
	s.SendServiceChecks(payload)
	s.SendJSONToV1Intake("test")

	f.AssertNotCalled(t, "SubmitV1Intake")
	f.AssertNotCalled(t, "SubmitEvents")
	f.AssertNotCalled(t, "SubmitV1CheckRuns")
	f.AssertNotCalled(t, "SubmitServiceChecks")
	f.AssertNotCalled(t, "SubmitV1Series")
	f.AssertNotCalled(t, "SubmitSeries")
	f.AssertNotCalled(t, "SubmitSketchSeries")

	// We never disable metadata
	f.On("SubmitV1Intake", payloads, jsonExtraHeaders).Return(nil).Times(1)
	s.SendMetadata(payload)
	f.AssertNumberOfCalls(t, "SubmitV1Intake", 1) // called once for the metadata
}
