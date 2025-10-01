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

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	metricscompression "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/def"
	metricscompressionimpl "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
	metricsserializer "github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
	"github.com/DataDog/datadog-agent/pkg/version"
)

func TestInitExtraHeadersNoopCompression(t *testing.T) {
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("serializer_compressor_kind", "blah")

	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	s := NewSerializer(nil, nil, compressor, mockConfig, logmock.New(t), "testhost")
	initExtraHeaders(s)

	expected := make(http.Header)
	expected.Set("Content-Type", jsonContentType)
	assert.Equal(t, expected, s.jsonExtraHeaders)

	expected = make(http.Header)
	expected.Set(payloadVersionHTTPHeader, version.AgentPayloadVersion)
	expected.Set("Content-Type", protobufContentType)
	assert.Equal(t, expected, s.protobufExtraHeaders)

	// No "Content-Encoding" header
	expected = make(http.Header)
	expected.Set("Content-Type", jsonContentType)
	expected.Set("Content-Encoding", "identity")
	assert.Equal(t, expected, s.jsonExtraHeadersWithCompression)

	expected = make(http.Header)
	expected.Set("Content-Type", protobufContentType)
	expected.Set("Content-Encoding", "identity")
	expected.Set(payloadVersionHTTPHeader, version.AgentPayloadVersion)
	assert.Equal(t, expected, s.protobufExtraHeadersWithCompression)
}

func TestInitExtraHeadersWithCompression(t *testing.T) {
	tests := map[string]struct {
		kind             string
		expectedEncoding string
	}{
		"zlib": {kind: compression.ZlibKind, expectedEncoding: compression.ZlibEncoding},
		"zstd": {kind: compression.ZstdKind, expectedEncoding: compression.ZstdEncoding},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
			s := NewSerializer(nil, nil, compressor, mockConfig, logmock.New(t), "testhost")
			initExtraHeaders(s)

			expected := make(http.Header)
			expected.Set("Content-Type", jsonContentType)
			assert.Equal(t, expected, s.jsonExtraHeaders)

			expected = make(http.Header)
			expected.Set("Content-Type", protobufContentType)
			expected.Set(payloadVersionHTTPHeader, version.AgentPayloadVersion)
			assert.Equal(t, expected, s.protobufExtraHeaders)

			// "Content-Encoding" header present with correct value
			expected = make(http.Header)
			expected.Set("Content-Type", jsonContentType)
			expected.Set("Content-Encoding", tc.expectedEncoding)
			assert.Equal(t, expected, s.jsonExtraHeadersWithCompression)

			expected = make(http.Header)
			expected.Set("Content-Type", protobufContentType)
			expected.Set("Content-Encoding", tc.expectedEncoding)
			expected.Set(payloadVersionHTTPHeader, version.AgentPayloadVersion)
			assert.Equal(t, expected, s.protobufExtraHeadersWithCompression)
		})

	}
}

func TestAgentPayloadVersion(t *testing.T) {
	assert.NotEmpty(t, version.AgentPayloadVersion, "version.AgentPayloadVersion is empty, indicates that the package was not built correctly")
}

var (
	jsonHeader     = []byte("{")
	jsonFooter     = []byte("}")
	jsonItem       = []byte("TO JSON")
	jsonString     = []byte("{TO JSON}")
	protobufString = []byte("TO PROTOBUF")
)

type testPayload struct {
	compressor metricscompression.Component
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
			fmt.Printf("Payload:  %q\nExpected: %q\n", string(payload), prefix)
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

func TestSendV1EventsNew(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			f := &forwarder.MockedForwarder{}

			compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
			s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")
			matcher := createJSONPayloadMatcher(`{"apiKey":"","events":{"api":[{"msg_title":"","msg_text":"","timestamp":0,"host":""}]},"internalHostname"`, s)
			f.On("SubmitV1Intake", matcher, s.jsonExtraHeadersWithCompression).Return(nil).Times(1)

			err := s.SendEvents([]*event.Event{{}})
			require.Nil(t, err)
			f.AssertExpectations(t)
		})
	}
}

func TestSendV1EventsNewNoEmpty(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)
			f := &forwarder.MockedForwarder{}

			compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
			s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")
			err := s.SendEvents([]*event.Event{})
			require.Nil(t, err)
			f.AssertNotCalled(t, "SubmitV1Events")
		})
	}
}

func TestSendV1ServiceChecks(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &forwarder.MockedForwarder{}
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)

			compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
			s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")
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
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &forwarder.MockedForwarder{}
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("use_v2_api.series", false)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)

			compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
			s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")
			matcher := createJSONPayloadMatcher(
				`{"series":[{"metric":"foo","points":[[1759241515,3.14],[1759241525,2.71]],`+
					`"tags":["bar","baz"],"host":"localhost","device":"sda","type":"gauge",`+
					`"interval":10,"source_type_name":"System"}]}`, s)

			f.On("SubmitV1Series", matcher, s.jsonExtraHeadersWithCompression).Return(nil).Times(1)

			err := s.SendIterableSeries(metricsserializer.CreateSerieSource(metrics.Series{&metrics.Serie{
				Name:   "foo",
				MType:  metrics.APIGaugeType,
				Device: "sda",
				Tags: tagset.NewCompositeTags(
					[]string{"bar"},
					[]string{"baz", "dd.internal.resource:ook:eek"},
				),
				Points: []metrics.Point{
					{Ts: 1759241515, Value: 3.14},
					{Ts: 1759241525, Value: 2.71},
				},
				Host:           "localhost",
				SourceTypeName: "System",
				Interval:       10,
			}}))
			require.Nil(t, err)
			f.AssertExpectations(t)
		})
	}
}

func TestSendSeries(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &forwarder.MockedForwarder{}
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("use_v2_api.series", true) // default value, but just to be sure
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)

			compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
			s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")
			matcher := createProtoscopeMatcher(`1: {
		1: { 1: {"host"} 2: {"localhost"} }
        1: { 1: {"device"} 2: {"sda"} }
        1: { 1: {"ook" } 2: {"eek"} }
        2: {"foo"}
        3: {"bar"} 3:{"baz"}
		5: 3
        7: {"System"}
        8: 10
        4: { 2: 1759241515 1: 3.14 }
        4: { 2: 1759241525 1: 2.71 }
		9: { 1: { 4: 10 }}
	  }`, s)
			f.On("SubmitSeries", matcher, s.protobufExtraHeadersWithCompression).Return(nil).Times(1)

			err := s.SendIterableSeries(metricsserializer.CreateSerieSource(metrics.Series{&metrics.Serie{
				Name:   "foo",
				MType:  metrics.APIGaugeType,
				Device: "sda",
				Tags: tagset.NewCompositeTags(
					[]string{"bar"},
					[]string{"baz", "dd.internal.resource:ook:eek"},
				),
				Points: []metrics.Point{
					{Ts: 1759241515, Value: 3.14},
					{Ts: 1759241525, Value: 2.71},
				},
				Host:           "localhost",
				SourceTypeName: "System",
				Interval:       10,
			}}))
			require.Nil(t, err)
			f.AssertExpectations(t)
		})
	}
}

func TestSendSketch(t *testing.T) {
	tests := map[string]struct {
		kind string
	}{
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &forwarder.MockedForwarder{}
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("use_v2_api.series", true) // default value, but just to be sure
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)

			compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
			s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")
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
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &forwarder.MockedForwarder{}
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)

			compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
			s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")
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
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			f := &forwarder.MockedForwarder{}
			payload := []byte("\"test\"")
			mockConfig := configmock.New(t)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)

			compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
			s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")
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
		"zlib": {kind: compression.ZlibKind},
		"zstd": {kind: compression.ZstdKind},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			mockConfig := configmock.New(t)

			mockConfig.SetWithoutSource("enable_payloads.events", false)
			mockConfig.SetWithoutSource("enable_payloads.series", false)
			mockConfig.SetWithoutSource("enable_payloads.service_checks", false)
			mockConfig.SetWithoutSource("enable_payloads.sketches", false)
			mockConfig.SetWithoutSource("enable_payloads.json_to_v1_intake", false)
			mockConfig.SetWithoutSource("serializer_compressor_kind", tc.kind)

			f := &forwarder.MockedForwarder{}

			compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
			s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

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

func TestSendIterableSeriesPreaggregationDualShip(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("use_v2_api.series", true)
	mockConfig.SetWithoutSource("preaggregation.enabled", true)
	// No allowlist should result in both destinations getting all metrics

	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

	series := metrics.Series{
		&metrics.Serie{Name: "cpu.usage", Host: "testhost"},
		&metrics.Serie{Name: "memory.usage", Host: "testhost"},
		&metrics.Serie{Name: "disk.io", Host: "testhost"},
	}

	var capturedPayloads transaction.BytesPayloads
	f.On("SubmitSeries", mock.MatchedBy(func(p transaction.BytesPayloads) bool {
		capturedPayloads = p
		return true
	}), s.protobufExtraHeadersWithCompression).Return(nil).Times(1)

	err := s.SendIterableSeries(metricsserializer.CreateSerieSource(series))
	require.Nil(t, err)
	f.AssertExpectations(t)

	require.Len(t, capturedPayloads, 2, "Should create exactly 2 payloads")

	payloadsByDestination := make(map[transaction.Destination]transaction.BytesPayload)
	for _, payload := range capturedPayloads {
		payloadsByDestination[payload.Destination] = *payload
	}

	assert.Len(t, payloadsByDestination, 2, "Should have exactly 2 unique destinations")

	allRegionsPayload, hasAllRegions := payloadsByDestination[transaction.AllRegions]
	preaggrOnlyPayload, hasPreaggrOnly := payloadsByDestination[transaction.PreaggrOnly]
	assert.True(t, hasAllRegions, "AllRegions destination should exist in dual-ship mode")
	assert.True(t, hasPreaggrOnly, "PreaggrOnly destination should exist in dual-ship mode")

	expectedMetrics := []string{"cpu.usage", "memory.usage", "disk.io"}
	assertPayloadContainsAllMetrics(t, allRegionsPayload, expectedMetrics, s, "AllRegions destination should contain all metrics in dual-ship mode")
	assertPayloadContainsAllMetrics(t, preaggrOnlyPayload, expectedMetrics, s, "PreaggrOnly destination should contain all metrics in dual-ship mode")
}

func TestSendIterableSeriesPreaggregationWithAllowlist(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("use_v2_api.series", true)
	mockConfig.SetWithoutSource("preaggregation.enabled", true)
	mockConfig.SetWithoutSource("preaggregation.metric_allowlist", []string{"cpu.usage", "memory.usage"})

	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

	series := metrics.Series{
		&metrics.Serie{Name: "cpu.usage", Host: "testhost"},
		&metrics.Serie{Name: "memory.usage", Host: "testhost"},
		&metrics.Serie{Name: "disk.io", Host: "testhost"},
	}

	var capturedPayloads transaction.BytesPayloads
	f.On("SubmitSeries", mock.MatchedBy(func(p transaction.BytesPayloads) bool {
		capturedPayloads = p
		return true
	}), s.protobufExtraHeadersWithCompression).Return(nil).Times(1)

	err := s.SendIterableSeries(metricsserializer.CreateSerieSource(series))
	require.Nil(t, err)
	f.AssertExpectations(t)

	require.Len(t, capturedPayloads, 2, "Should create exactly 2 payloads")

	payloadsByDestination := make(map[transaction.Destination]transaction.BytesPayload)
	for _, payload := range capturedPayloads {
		payloadsByDestination[payload.Destination] = *payload
	}

	assert.Len(t, payloadsByDestination, 2, "Should have exactly 2 unique destinations")

	allRegionsPayload, hasAllRegions := payloadsByDestination[transaction.AllRegions]
	preaggrOnlyPayload, hasPreaggrOnly := payloadsByDestination[transaction.PreaggrOnly]
	assert.True(t, hasAllRegions, "AllRegions destination should exist for split routing")
	assert.True(t, hasPreaggrOnly, "PreaggrOnly destination should exist for split routing")

	allowlistMetrics := []string{"cpu.usage", "memory.usage"}
	nonAllowlistMetrics := []string{"disk.io"}

	assertPayloadContainsAllMetrics(t, allRegionsPayload, nonAllowlistMetrics, s, "AllRegions should contain non-allowlist metrics")
	assertPayloadContainsNoMetrics(t, allRegionsPayload, allowlistMetrics, s, "AllRegions should NOT contain allowlist metrics")

	assertPayloadContainsAllMetrics(t, preaggrOnlyPayload, allowlistMetrics, s, "PreaggrOnly should contain allowlist metrics")
	assertPayloadContainsNoMetrics(t, preaggrOnlyPayload, nonAllowlistMetrics, s, "PreaggrOnly should NOT contain non-allowlist metrics")
}

func TestSendIterableSeriesFailoverBypassesPreaggregation(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("use_v2_api.series", true)
	mockConfig.SetWithoutSource("multi_region_failover.enabled", true)
	mockConfig.SetWithoutSource("multi_region_failover.failover_metrics", true)
	mockConfig.SetWithoutSource("multi_region_failover.metric_allowlist", []string{"failover.metric"})
	mockConfig.SetWithoutSource("preaggregation.enabled", true)
	mockConfig.SetWithoutSource("preaggregation.metric_allowlist", []string{"preaggr.metric"})

	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

	series := metrics.Series{
		&metrics.Serie{Name: "failover.metric", Host: "testhost"},
		&metrics.Serie{Name: "preaggr.metric", Host: "testhost"},
		&metrics.Serie{Name: "regular.metric", Host: "testhost"},
	}

	var capturedPayloads transaction.BytesPayloads
	f.On("SubmitSeries", mock.MatchedBy(func(p transaction.BytesPayloads) bool {
		capturedPayloads = p
		return true
	}), s.protobufExtraHeadersWithCompression).Return(nil)

	err := s.SendIterableSeries(metricsserializer.CreateSerieSource(series))
	require.Nil(t, err)
	f.AssertExpectations(t)

	require.Len(t, capturedPayloads, 2, "Should create exactly 2 payloads")

	payloadsByDestination := make(map[transaction.Destination]transaction.BytesPayload)
	for _, payload := range capturedPayloads {
		payloadsByDestination[payload.Destination] = *payload
	}

	assert.Len(t, payloadsByDestination, 2, "Should have exactly 2 unique destinations")

	_, hasPreaggrOnly := payloadsByDestination[transaction.PreaggrOnly]
	assert.False(t, hasPreaggrOnly, "Failover should bypass preaggregation - PreaggrOnly destination should not exist")
}

func TestSendIterableSeriesPreaggregationEmptyAllowlist(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("use_v2_api.series", true)
	mockConfig.SetWithoutSource("preaggregation.enabled", true)
	mockConfig.SetWithoutSource("preaggregation.metric_allowlist", []string{}) // Empty allowlist triggers dual-ship

	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

	series := metrics.Series{
		&metrics.Serie{Name: "cpu.usage", Host: "testhost"},
		&metrics.Serie{Name: "memory.usage", Host: "testhost"},
	}

	var capturedPayloads transaction.BytesPayloads
	f.On("SubmitSeries", mock.MatchedBy(func(p transaction.BytesPayloads) bool {
		capturedPayloads = p
		return true
	}), s.protobufExtraHeadersWithCompression).Return(nil).Times(1)

	err := s.SendIterableSeries(metricsserializer.CreateSerieSource(series))
	require.Nil(t, err)
	f.AssertExpectations(t)

	require.Len(t, capturedPayloads, 2, "Should create exactly 2 payloads")

	payloadsByDestination := make(map[transaction.Destination]transaction.BytesPayload)
	for _, payload := range capturedPayloads {
		payloadsByDestination[payload.Destination] = *payload
	}

	assert.Len(t, payloadsByDestination, 2, "Should have exactly 2 unique destinations")

	allRegionsPayload, hasAllRegions := payloadsByDestination[transaction.AllRegions]
	preaggrOnlyPayload, hasPreaggrOnly := payloadsByDestination[transaction.PreaggrOnly]
	assert.True(t, hasAllRegions, "AllRegions destination should exist with empty allowlist")
	assert.True(t, hasPreaggrOnly, "PreaggrOnly destination should exist with empty allowlist")

	expectedMetrics := []string{"cpu.usage", "memory.usage"}
	assertPayloadContainsAllMetrics(t, allRegionsPayload, expectedMetrics, s, "AllRegions destination should contain all metrics with empty allowlist")
	assertPayloadContainsAllMetrics(t, preaggrOnlyPayload, expectedMetrics, s, "PreaggrOnly destination should contain all metrics with empty allowlist")
}

func TestSendIterableSeriesPreaggregationDisabled(t *testing.T) {
	f := &forwarder.MockedForwarder{}
	mockConfig := configmock.New(t)
	mockConfig.SetWithoutSource("use_v2_api.series", true)
	mockConfig.SetWithoutSource("preaggregation.enabled", false)

	compressor := metricscompressionimpl.NewCompressorReq(metricscompressionimpl.Requires{Cfg: mockConfig}).Comp
	s := NewSerializer(f, nil, compressor, mockConfig, logmock.New(t), "testhost")

	series := metrics.Series{
		&metrics.Serie{Name: "cpu.usage", Host: "testhost"},
		&metrics.Serie{Name: "memory.usage", Host: "testhost"},
	}

	var capturedPayloads transaction.BytesPayloads
	f.On("SubmitSeries", mock.MatchedBy(func(p transaction.BytesPayloads) bool {
		capturedPayloads = p
		return true
	}), s.protobufExtraHeadersWithCompression).Return(nil).Times(1)

	err := s.SendIterableSeries(metricsserializer.CreateSerieSource(series))
	require.Nil(t, err)
	f.AssertExpectations(t)

	require.Len(t, capturedPayloads, 1, "Should create exactly 1 payload")

	payloadsByDestination := make(map[transaction.Destination]transaction.BytesPayload)
	for _, payload := range capturedPayloads {
		payloadsByDestination[payload.Destination] = *payload
	}

	assert.Len(t, payloadsByDestination, 1, "Should have exactly 1 unique destination")

	allRegionsPayload, hasAllRegions := payloadsByDestination[transaction.AllRegions]
	_, hasPreaggrOnly := payloadsByDestination[transaction.PreaggrOnly]
	assert.True(t, hasAllRegions, "AllRegions destination should exist when preaggregation is disabled")
	assert.False(t, hasPreaggrOnly, "PreaggrOnly destination should not exist when preaggregation is disabled")

	expectedMetrics := []string{"cpu.usage", "memory.usage"}
	assertPayloadContainsAllMetrics(t, allRegionsPayload, expectedMetrics, s, "AllRegions should contain all metrics when preaggregation is disabled")
}

func assertPayloadContainsAllMetrics(t *testing.T, payload transaction.BytesPayload, metricNames []string, s *Serializer, msgAndArgs ...any) {
	content := payload.GetContent()
	if decompressed, err := s.Strategy.Decompress(content); err == nil {
		decompressedStr := string(decompressed)
		for _, metricName := range metricNames {
			assert.Contains(t, decompressedStr, metricName, msgAndArgs...)
		}
	} else {
		assert.Fail(t, "Failed to decompress payload", msgAndArgs...)
	}
}

func assertPayloadContainsNoMetrics(t *testing.T, payload transaction.BytesPayload, metricNames []string, s *Serializer, msgAndArgs ...any) {
	content := payload.GetContent()
	if decompressed, err := s.Strategy.Decompress(content); err == nil {
		decompressedStr := string(decompressed)
		for _, metricName := range metricNames {
			assert.NotContains(t, decompressedStr, metricName, msgAndArgs...)
		}
	} else {
		assert.Fail(t, "Failed to decompress payload", msgAndArgs...)
	}
}
