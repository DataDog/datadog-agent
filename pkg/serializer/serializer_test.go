package serializer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
}

var (
	jsonPayloads     = forwarder.Payloads{}
	protobufPayloads = forwarder.Payloads{}
	jsonString       = []byte("TO JSON")
	protobufString   = []byte("TO PROTOBUF")
)

func init() {
	jsonPayloads, _ = mkPayloads(jsonString, true)
	protobufPayloads, _ = mkPayloads(protobufString, false)
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
	f.On("SubmitV1Intake", jsonPayloads, jsonExtraHeaders).Return(nil).Times(1)

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

func TestSendSeries(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	f.On("SubmitV1Series", jsonPayloads, jsonExtraHeaders).Return(nil).Times(1)

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
	f.On("SubmitSketchSeries", protobufPayloads, protobufExtraHeaders).Return(nil).Times(1)

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
