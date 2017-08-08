package serializer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

func TestInit(t *testing.T) {
	assert.Equal(t, map[string]string{"Content-Type": jsonContentType}, jsonExtraHeaders)

	assert.Equal(t,
		map[string]string{
			payloadVersionHTTPHeader: "",
			"Content-Type":           protobufContentType,
		},
		protobufExtraHeaders)

	if compression.ContentEncoding == "" {
		assert.Equal(t, map[string]string{"Content-Type": jsonContentType}, jsonExtraHeadersWithCompression)
		assert.Equal(t,
			map[string]string{
				payloadVersionHTTPHeader: "",
				"Content-Type":           protobufContentType,
			},
			protobufExtraHeadersWithCompression)
	} else {
		assert.Equal(t,
			map[string]string{
				"Content-Type":     jsonContentType,
				"Content-Encoding": compression.ContentEncoding,
			},
			jsonExtraHeadersWithCompression)
		assert.Equal(t,
			map[string]string{
				payloadVersionHTTPHeader: "",
				"Content-Type":           protobufContentType,
				"Content-Encoding":       compression.ContentEncoding,
			},
			protobufExtraHeadersWithCompression)
	}
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

func TestSendEvents(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	f.On("SubmitV1Intake", jsonPayloads, jsonExtraHeadersWithCompression).Return(nil).Times(1)

	s := Serializer{Forwarder: f}

	payload := &testPayload{}
	err := s.SendEvents(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendEvents(errPayload)
	require.NotNil(t, err)
}

func TestSendV2Events(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	f.On("SubmitEvents", protobufPayloads, protobufExtraHeadersWithCompression).Return(nil).Times(1)
	config.Datadog.Set("use_v2_endpoint.events", true)
	defer config.Datadog.Set("use_v2_endpoint.events", nil)

	s := Serializer{Forwarder: f}

	payload := &testPayload{}
	err := s.SendEvents(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendEvents(errPayload)
	require.NotNil(t, err)
}

func TestSendServiceChecks(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	payloads, _ := mkPayloads(jsonString, false)
	f.On("SubmitV1CheckRuns", payloads, jsonExtraHeaders).Return(nil).Times(1)

	s := Serializer{Forwarder: f}

	payload := &testPayload{}
	err := s.SendServiceChecks(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendServiceChecks(errPayload)
	require.NotNil(t, err)
}

func TestSendV2ServiceChecks(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	payloads, _ := mkPayloads(protobufString, false)
	f.On("SubmitServiceChecks", payloads, protobufExtraHeaders).Return(nil).Times(1)
	config.Datadog.Set("use_v2_endpoint.service_checks", true)
	defer config.Datadog.Set("use_v2_endpoint.service_checks", nil)

	s := Serializer{Forwarder: f}

	payload := &testPayload{}
	err := s.SendServiceChecks(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendServiceChecks(errPayload)
	require.NotNil(t, err)
}

func TestSendSeries(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	f.On("SubmitV1Series", jsonPayloads, jsonExtraHeadersWithCompression).Return(nil).Times(1)

	s := Serializer{Forwarder: f}

	payload := &testPayload{}
	err := s.SendSeries(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendSeries(errPayload)
	require.NotNil(t, err)
}

func TestSendV2Series(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	f.On("SubmitSeries", protobufPayloads, protobufExtraHeadersWithCompression).Return(nil).Times(1)
	config.Datadog.Set("use_v2_endpoint.series", true)
	defer config.Datadog.Set("use_v2_endpoint.series", nil)

	s := Serializer{Forwarder: f}

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

	s := Serializer{Forwarder: f}

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

	s := Serializer{Forwarder: f}

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

	s := Serializer{Forwarder: f}

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
