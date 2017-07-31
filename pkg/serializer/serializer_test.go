package serializer

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/forwarder"
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
	jsonString     = []byte("TO JSON")
	protobufString = []byte("TO PROTOBUF")
)

type testPayload struct{}

func (p *testPayload) MarshalJSON() ([]byte, error) { return jsonString, nil }
func (p *testPayload) Marshal() ([]byte, error)     { return protobufString, nil }

type testErrorPayload struct{}

func (p *testErrorPayload) MarshalJSON() ([]byte, error) { return nil, fmt.Errorf("some error") }
func (p *testErrorPayload) Marshal() ([]byte, error)     { return nil, fmt.Errorf("some error") }

func TestSendEvents(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	f.On("SubmitV1Intake", &jsonString, jsonExtraHeaders).Return(nil).Times(1)

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
	f.On("SubmitV1CheckRuns", &jsonString, jsonExtraHeaders).Return(nil).Times(1)

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
	f.On("SubmitV1Series", &jsonString, jsonExtraHeaders).Return(nil).Times(1)

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
	f.On("SubmitSketchSeries", &protobufString, protobufExtraHeaders).Return(nil).Times(1)

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
	f.On("SubmitV1Intake", &jsonString, jsonExtraHeaders).Return(nil).Times(1)

	s := Serializer{Forwarder: f}

	payload := &testPayload{}
	err := s.SendMetadata(payload)
	require.Nil(t, err)
	f.AssertExpectations(t)

	f.On("SubmitV1Intake", &jsonString, jsonExtraHeaders).Return(fmt.Errorf("some error")).Times(1)
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
	f.On("SubmitV1Intake", &payload, jsonExtraHeaders).Return(nil).Times(1)

	s := Serializer{Forwarder: f}

	err := s.SendJSONToV1Intake("test")
	require.Nil(t, err)
	f.AssertExpectations(t)

	f.On("SubmitV1Intake", &payload, jsonExtraHeaders).Return(fmt.Errorf("some error")).Times(1)
	err = s.SendJSONToV1Intake("test")
	require.NotNil(t, err)
	f.AssertExpectations(t)

	errPayload := &testErrorPayload{}
	err = s.SendJSONToV1Intake(errPayload)
	require.NotNil(t, err)
}
