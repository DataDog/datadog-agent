// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test && zlib && zstd

package serializer

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/protocolbuffers/protoscope"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/comp/serializer/compression"
	"github.com/DataDog/datadog-agent/comp/serializer/compression/compressionimpl"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	metricsserializer "github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
)

func TestInitExtraHeadersNoopCompression(t *testing.T) {
	mockConfig := pkgconfigsetup.Conf()
	mockConfig.SetWithoutSource("serializer_compressor_kind", "blah")
	s := NewSerializer(nil, nil, compressionimpl.NewCompressor(mockConfig), mockConfig, "testhost")
	initExtraHeaders(s)

	expected := make(http.Header)
	expected.Set("Content-Type", jsonContentType)
	assert.Equal(t, expected, s.jsonExtraHeaders)

	expected = make(http.Header)
	expected.Set(payloadVersionHTTPHeader, AgentPayloadVersion)
	expected.Set("Content-Type", protobufContentType)
	assert.Equal(t, expected, s.protobufExtraHeaders)

	// No "Content-Encoding" header
	expected = make(http.Header)
	expected.Set("Content-Type", jsonContentType)
	assert.Equal(t, expected, s.jsonExtraHeadersWithCompression)

	expected = make(http.Header)
	expected.Set("Content-Type", protobufContentType)
	expected.Set(payloadVersionHTTPHeader, AgentPayloadVersion)
	assert.Equal(t, expected, s.protobufExtraHeadersWithCompression)
}

func TestInitExtraHeadersWithCompression(t *testing.T) {
	tests := map[string]struct {
		kind             string
		expectedEncoding string
	}{
		"zlib": {kind: compressionimpl.ZlibKind, expectedEncoding: compression.ZlibEncoding},
		"zstd": {kind: compressionimpl.ZstdKind, expectedEncoding: compression.ZstdEncoding},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			s := NewSerializer(nil, nil, compressionimpl.NewCompressor(mockConfig), mockConfig, "testhost")
			initExtraHeaders(s)

			expected := make(http.Header)
			expected.Set("Content-Type", jsonContentType)
			assert.Equal(t, expected, s.jsonExtraHeaders)

			expected = make(http.Header)
			expected.Set("Content-Type", protobufContentType)
			expected.Set(payloadVersionHTTPHeader, AgentPayloadVersion)
			assert.Equal(t, expected, s.protobufExtraHeaders)

			// "Content-Encoding" header present with correct value
			expected = make(http.Header)
			expected.Set("Content-Type", jsonContentType)
			expected.Set("Content-Encoding", tc.expectedEncoding)
			assert.Equal(t, expected, s.jsonExtraHeadersWithCompression)

			expected = make(http.Header)
			expected.Set("Content-Type", protobufContentType)
			expected.Set("Content-Encoding", tc.expectedEncoding)
			expected.Set(payloadVersionHTTPHeader, AgentPayloadVersion)
			assert.Equal(t, expected, s.protobufExtraHeadersWithCompression)
		})

	}
}

func TestAgentPayloadVersion(t *testing.T) {
	assert.NotEmpty(t, AgentPayloadVersion, "AgentPayloadVersion is empty, indicates that the package was not built correctly")
}

var (
	jsonHeader     = []byte("{")
	jsonFooter     = []byte("}")
	jsonItem       = []byte("TO JSON")
	jsonString     = []byte("{TO JSON}")
	protobufString = []byte("TO PROTOBUF")
)

type testPayload struct {
	compressor compression.Component
}

//nolint:revive // TODO(AML) Fix revive linter
func (p *testPayload) MarshalJSON() ([]byte, error) { return jsonString, nil }

//nolint:revive // TODO(AML) Fix revive linter
func (p *testPayload) Marshal() ([]byte, error) { return protobufString, nil }

//nolint:revive // TODO(AML) Fix revive linter
func (p *testPayload) MarshalSplitCompress(bufferContext *marshaler.BufferContext) (transaction.BytesPayloads, error) {
	payloads := transaction.BytesPayloads{}
	payload, err := p.compressor.Compress(protobufString)
	if err != nil {
		return nil, err
	}
	payloads = append(payloads, transaction.NewBytesPayloadWithoutMetaData(payload))
	return payloads, nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (p *testPayload) SplitPayload(int) ([]marshaler.AbstractMarshaler, error) {
	return []marshaler.AbstractMarshaler{}, nil
}

//nolint:revive // TODO(AML) Fix revive linter
func (p *testPayload) WriteHeader(stream *jsoniter.Stream) error {
	_, err := stream.Write(jsonHeader)
	return err
}

//nolint:revive // TODO(AML) Fix revive linter
func (p *testPayload) WriteFooter(stream *jsoniter.Stream) error {
	_, err := stream.Write(jsonFooter)
	return err
}

//nolint:revive // TODO(AML) Fix revive linter
func (p *testPayload) WriteItem(stream *jsoniter.Stream, i int) error {
	_, err := stream.Write(jsonItem)
	return err
}

//nolint:revive // TODO(AML) Fix revive linter
func (p *testPayload) Len() int { return 1 }

//nolint:revive // TODO(AML) Fix revive linter
func (p *testPayload) DescribeItem(i int) string { return "description" }

type testErrorPayload struct{}

//nolint:revive // TODO(AML) Fix revive linter
func (p *testErrorPayload) MarshalJSON() ([]byte, error) { return nil, fmt.Errorf("some error") }

//nolint:revive // TODO(AML) Fix revive linter
func (p *testErrorPayload) Marshal() ([]byte, error) { return nil, fmt.Errorf("some error") }

//nolint:revive // TODO(AML) Fix revive linter
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

//nolint:revive // TODO(AML) Fix revive linter
func (p *testErrorPayload) WriteItem(stream *jsoniter.Stream, i int) error {
	return fmt.Errorf("some error")
}
func (p *testErrorPayload) Len() int { return 1 }

//nolint:revive // TODO(AML) Fix revive linter
func (p *testErrorPayload) DescribeItem(i int) string { return "description" }

func mkPayloads(payload []byte, compress bool, s *Serializer) (transaction.BytesPayloads, error) {
	payloads := transaction.BytesPayloads{}
	var err error
	if compress {
		payload, err = s.Strategy.Compress(payload)
		if err != nil {
			return nil, err
		}
	}
	payloads = append(payloads, transaction.NewBytesPayloadWithoutMetaData(payload))
	return payloads, nil
}

func createJSONPayloadMatcher(prefix string, s *Serializer) interface{} {
	return mock.MatchedBy(func(payloads transaction.BytesPayloads) bool {
		return doPayloadsMatch(payloads, prefix, s)
	})
}

func doPayloadsMatch(payloads transaction.BytesPayloads, prefix string, s *Serializer) bool {
	for _, compressedPayload := range payloads {
		if payload, err := s.Strategy.Decompress(compressedPayload.GetContent()); err != nil {
			return false
		} else { //nolint:revive // TODO(AML) Fix revive linter
			if strings.HasPrefix(string(payload), prefix) {
				return true
			}
		}
	}
	return false
}

func createProtoscopeMatcher(protoscopeDef string, s *Serializer) interface{} {
	return mock.MatchedBy(func(payloads transaction.BytesPayloads) bool {
		for _, compressedPayload := range payloads {
			if payload, err := s.Strategy.Decompress(compressedPayload.GetContent()); err != nil {
				return false
			} else { //nolint:revive // TODO(AML) Fix revive linter
				res, err := protoscope.NewScanner(protoscopeDef).Exec()
				if err != nil {
					return false
				}
				if reflect.DeepEqual(res, payload) {
					return true
				} else { //nolint:revive // TODO(AML) Fix revive linter
					fmt.Printf("Did not match. Payload was\n%x and protoscope compilation was\n%x\n", payload, res)
				}
			}
		}
		return false
	})
}

func TestSendV1Events(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compressionimpl.ZlibKind},
		"zstd": {kind: compressionimpl.ZstdKind},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("enable_events_stream_payload_serialization", false)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			f := &forwarder.MockedForwarder{}

			s := NewSerializer(f, nil, compressionimpl.NewCompressor(mockConfig), mockConfig, "testhost")
			matcher := createJSONPayloadMatcher(`{"apiKey":"","events":{},"internalHostname"`, s)
			f.On("SubmitV1Intake", matcher, s.jsonExtraHeadersWithCompression).Return(nil).Times(1)

			err := s.SendEvents([]*event.Event{})
			require.Nil(t, err)
			f.AssertExpectations(t)
		})
	}
}

func TestSendV1EventsCreateMarshalersBySourceType(t *testing.T) {

	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compressionimpl.ZlibKind},
		"zstd": {kind: compressionimpl.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("enable_events_stream_payload_serialization", true)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			f := &forwarder.MockedForwarder{}

			s := NewSerializer(f, nil, compressionimpl.NewCompressor(mockConfig), mockConfig, "testhost")

			events := event.Events{&event.Event{SourceTypeName: "source1"}, &event.Event{SourceTypeName: "source2"}, &event.Event{SourceTypeName: "source3"}}
			payloadsCountMatcher := func(payloadCount int) interface{} {
				return mock.MatchedBy(func(payloads transaction.BytesPayloads) bool {
					return len(payloads) == payloadCount
				})
			}

			f.On("SubmitV1Intake", payloadsCountMatcher(1), s.jsonExtraHeadersWithCompression).Return(nil)
			err := s.SendEvents(events)
			assert.NoError(t, err)
			f.AssertExpectations(t)

			mockConfig.SetWithoutSource("serializer_max_payload_size", 20)

			f.On("SubmitV1Intake", payloadsCountMatcher(3), s.jsonExtraHeadersWithCompression).Return(nil)
			err = s.SendEvents(events)
			assert.NoError(t, err)
			f.AssertExpectations(t)
		})
	}
}

func TestSendV1ServiceChecks(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compressionimpl.ZlibKind},
		"zstd": {kind: compressionimpl.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &forwarder.MockedForwarder{}
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("enable_service_checks_stream_payload_serialization", false)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			s := NewSerializer(f, nil, compressionimpl.NewCompressor(mockConfig), mockConfig, "testhost")
			matcher := createJSONPayloadMatcher(`[{"check":"","host_name":"","timestamp":0,"status":0,"message":"","tags":null}]`, s)
			f.On("SubmitV1CheckRuns", matcher, s.jsonExtraHeadersWithCompression).Return(nil).Times(1)

			err := s.SendServiceChecks(servicecheck.ServiceChecks{&servicecheck.ServiceCheck{}})
			require.Nil(t, err)
			f.AssertExpectations(t)
		})
	}
}

func TestSendV1Series(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compressionimpl.ZlibKind},
		"zstd": {kind: compressionimpl.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &forwarder.MockedForwarder{}
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("enable_stream_payload_serialization", false)
			mockConfig.SetWithoutSource("use_v2_api.series", false)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			s := NewSerializer(f, nil, compressionimpl.NewCompressor(mockConfig), mockConfig, "testhost")
			matcher := createJSONPayloadMatcher(`{"series":[]}`, s)

			f.On("SubmitV1Series", matcher, s.jsonExtraHeadersWithCompression).Return(nil).Times(1)

			err := s.SendIterableSeries(metricsserializer.CreateSerieSource(metrics.Series{}))
			require.Nil(t, err)
			f.AssertExpectations(t)
		})
	}
}

func TestSendSeries(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compressionimpl.ZlibKind},
		"zstd": {kind: compressionimpl.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &forwarder.MockedForwarder{}
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("use_v2_api.series", true) // default value, but just to be sure
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			s := NewSerializer(f, nil, compressionimpl.NewCompressor(mockConfig), mockConfig, "testhost")
			matcher := createProtoscopeMatcher(`1: {
		1: { 1: {"host"} }
		5: 3
		9: { 1: { 4: 10 }}
	  }`, s)
			f.On("SubmitSeries", matcher, s.protobufExtraHeadersWithCompression).Return(nil).Times(1)

			err := s.SendIterableSeries(metricsserializer.CreateSerieSource(metrics.Series{&metrics.Serie{}}))
			require.Nil(t, err)
			f.AssertExpectations(t)
		})
	}
}

func TestSendSketch(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compressionimpl.ZlibKind},
		"zstd": {kind: compressionimpl.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &forwarder.MockedForwarder{}
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("use_v2_api.series", true) // default value, but just to be sure
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			s := NewSerializer(f, nil, compressionimpl.NewCompressor(mockConfig), mockConfig, "testhost")
			matcher := createProtoscopeMatcher(`
		1: { 1: {"fakename"} 2: {"fakehost"} 8: { 1: { 4: 10 }}}
		2: {}
		`, s)
			f.On("SubmitSketchSeries", matcher, s.protobufExtraHeadersWithCompression).Return(nil).Times(1)

			err := s.SendSketch(metrics.NewSketchesSourceTestWithSketch())
			require.Nil(t, err)
			f.AssertExpectations(t)
		})
	}

}

func TestSendMetadata(t *testing.T) {

	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compressionimpl.ZlibKind},
		"zstd": {kind: compressionimpl.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &forwarder.MockedForwarder{}
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			compressor := compressionimpl.NewCompressor(mockConfig)
			s := NewSerializer(f, nil, compressor, mockConfig, "testhost")
			jsonPayloads, _ := mkPayloads(jsonString, true, s)
			f.On("SubmitMetadata", jsonPayloads, s.jsonExtraHeadersWithCompression).Return(nil).Times(1)

			payload := &testPayload{compressor: compressor}
			err := s.SendMetadata(payload)
			require.Nil(t, err)
			f.AssertExpectations(t)

			f.On("SubmitMetadata", jsonPayloads, s.jsonExtraHeadersWithCompression).Return(fmt.Errorf("some error")).Times(1)
			err = s.SendMetadata(payload)
			require.NotNil(t, err)
			f.AssertExpectations(t)

			errPayload := &testErrorPayload{}
			err = s.SendMetadata(errPayload)
			require.NotNil(t, err)
		})
	}
}

func TestSendProcessesMetadata(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compressionimpl.ZlibKind},
		"zstd": {kind: compressionimpl.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &forwarder.MockedForwarder{}
			payload := []byte("\"test\"")
			mockConfig := pkgconfigsetup.Conf()
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			s := NewSerializer(f, nil, compressionimpl.NewCompressor(mockConfig), mockConfig, "testhost")
			payloads, _ := mkPayloads(payload, true, s)
			f.On("SubmitV1Intake", payloads, s.jsonExtraHeadersWithCompression).Return(nil).Times(1)

			err := s.SendProcessesMetadata("test")
			require.Nil(t, err)
			f.AssertExpectations(t)

			f.On("SubmitV1Intake", payloads, s.jsonExtraHeadersWithCompression).Return(fmt.Errorf("some error")).Times(1)
			err = s.SendProcessesMetadata("test")
			require.NotNil(t, err)
			f.AssertExpectations(t)

			errPayload := &testErrorPayload{}
			err = s.SendProcessesMetadata(errPayload)
			require.NotNil(t, err)
		})
	}
}

func TestSendWithDisabledKind(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compressionimpl.ZlibKind},
		"zstd": {kind: compressionimpl.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := pkgconfigsetup.Conf()

			mockConfig.SetWithoutSource("enable_payloads.events", false)
			mockConfig.SetWithoutSource("enable_payloads.series", false)
			mockConfig.SetWithoutSource("enable_payloads.service_checks", false)
			mockConfig.SetWithoutSource("enable_payloads.sketches", false)
			mockConfig.SetWithoutSource("enable_payloads.json_to_v1_intake", false)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)

			f := &forwarder.MockedForwarder{}
			s := NewSerializer(f, nil, compressionimpl.NewCompressor(mockConfig), mockConfig, "testhost")
			jsonPayloads, _ := mkPayloads(jsonString, true, s)
			payload := &testPayload{}

			s.SendEvents(make(event.Events, 0))
			s.SendIterableSeries(metricsserializer.CreateSerieSource(metrics.Series{}))
			s.SendSketch(metrics.NewSketchesSourceTest())
			s.SendServiceChecks(make(servicecheck.ServiceChecks, 0))
			s.SendProcessesMetadata("test")

			f.AssertNotCalled(t, "SubmitMetadata")
			f.AssertNotCalled(t, "SubmitV1CheckRuns")
			f.AssertNotCalled(t, "SubmitV1Series")
			f.AssertNotCalled(t, "SubmitSketchSeries")

			// We never disable metadata
			f.On("SubmitMetadata", jsonPayloads, s.jsonExtraHeadersWithCompression).Return(nil).Times(1)
			s.SendMetadata(payload)
			f.AssertNumberOfCalls(t, "SubmitMetadata", 1) // called once for the metadata
		})
	}
}
