// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && zlib && zstd

package serializer

import (
	"bytes"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	secretnooptypes "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl/types"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	forwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	metricscompressionimpl "github.com/DataDog/datadog-agent/comp/serializer/metricscompression/impl"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	metricsserializer "github.com/DataDog/datadog-agent/pkg/serializer/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/tagset"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

type liveTestSource struct {
	values []*metrics.Serie
	index  int
	calls  int
}

func (s *liveTestSource) MoveNext() bool          { s.calls++; s.index++; return s.index < len(s.values) }
func (s *liveTestSource) Current() *metrics.Serie { return s.values[s.index] }
func (s *liveTestSource) Count() uint64           { return uint64(len(s.values)) }

type liveTestForwarder struct {
	forwarder.Forwarder
	mu       sync.Mutex
	payloads []*transaction.BytesPayload
	wire     []string
	err      error
}

func (f *liveTestForwarder) record(p *transaction.BytesPayload, endpoint string, headers http.Header) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.payloads = append(f.payloads, p)
	f.wire = append(f.wire, endpoint+"\x00"+string(p.GetContent()))
	for key := range headers {
		if strings.Contains(strings.ToLower(key), "capture") {
			panic("capture metadata escaped into headers")
		}
	}
	return f.err
}

func (f *liveTestForwarder) SubmitV1Series(payloads transaction.BytesPayloads, headers http.Header) error {
	for range f.GetDomainResolvers() {
		for _, p := range payloads {
			if err := f.record(p, endpoints.V1SeriesEndpoint.Route, headers); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *liveTestForwarder) SubmitTransaction(tx *transaction.HTTPTransaction) error {
	return f.record(tx.Payload, tx.Endpoint.Route, tx.Headers)
}

func liveManager(t *testing.T) (*telemetrycapture.Manager, telemetrycapture.Control) {
	t.Helper()
	m := telemetrycapture.NewManager("core-agent", "fixture", "fixture")
	t.Cleanup(m.Close)
	require.NoError(t, m.Register(telemetrycapture.Capability{Stream: telemetrycapture.Metrics, Cadence: 15 * time.Second}))
	control := telemetrycapture.Control{ProtocolVersion: telemetrycapture.ProtocolVersion, SessionID: "metric-capture-session"}
	_, err := m.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Metrics}})
	require.NoError(t, err)
	_, err = m.Activate(control)
	require.NoError(t, err)
	return m, control
}

func liveSerializer(t *testing.T, version int) (*Serializer, *liveTestForwarder) {
	t.Helper()
	cfg := configmock.New(t)
	cfg.SetInTest("dd_url", "https://primary.test")
	cfg.SetInTest("api_key", "secret-native-key")
	cfg.SetInTest("additional_endpoints", map[string][]string{"https://additional.test": {"second-native-key"}})
	cfg.SetInTest("use_v2_api.series", version != 1)
	cfg.SetInTest("use_v3_api.series.enabled", strconv.FormatBool(version == 3))
	cfg.SetInTest("serializer_compressor_kind", "zstd")
	cfg.SetInTest("serializer_max_series_points_per_payload", 2)
	cfg.SetInTest("serializer_max_uncompressed_payload_size", 420)
	cfg.SetInTest("enable_json_stream_shared_compressor_buffers", true)
	logger := logmock.New(t)
	base, err := forwarderimpl.NewTestForwarder(forwarder.Params{}, cfg, logger, &secretnooptypes.SecretNoop{})
	require.NoError(t, err)
	f := &liveTestForwarder{Forwarder: base}
	compressor := metricscompressionimpl.NewComponent(metricscompressionimpl.Requires{Cfg: cfg}).Comp
	s := NewSerializer(f, nil, compressor, cfg, logger, "native-host")
	s.LiveCaptureCadence = 15 * time.Second
	return s, f
}

func liveInput() *liveTestSource {
	return &liveTestSource{index: -1, values: []*metrics.Serie{
		{Name: "system.cpu.user", Host: "native-host", Source: metrics.MetricSource(123), Unit: "percent", SourceTypeName: "System", Resources: []metrics.Resource{{Type: "device", Name: "native-resource"}}, MType: metrics.APIGaugeType, Interval: 15, Tags: tagset.NewCompositeTags([]string{"device:native-device", "native:tag"}, nil), Points: []metrics.Point{{Ts: 1234567890.125, Value: 7.5}}},
		{Name: "uncaptured.metric", Host: "native-host", MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: 1234567890, Value: 8}}},
		{Name: "system.mem.total", Host: "native-host", MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: 1234567890.25, Value: 42}}},
		{Name: "system.uptime", Host: "native-host", MType: metrics.APIGaugeType, Points: []metrics.Point{{Ts: 1234567890.5, Value: 99}}},
		{Name: "system.disk.total", Host: "native-host", MType: metrics.APIGaugeType, Tags: tagset.NewCompositeTags([]string{strings.Repeat("oversized", 4096)}, nil), Points: []metrics.Point{{Ts: 1, Value: 1}, {Ts: 2, Value: 2}, {Ts: 3, Value: 3}}},
	}}
}

func TestLiveMetricCapturePreservesDelivery(t *testing.T) {
	for _, version := range []int{1, 2, 3} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			s, f := liveSerializer(t, version)
			baseline := liveInput()
			require.NoError(t, s.SendIterableSeries(baseline))
			require.Equal(t, len(baseline.values)+1, baseline.calls)
			wire := slices.Clone(f.wire)
			f.wire, f.payloads = nil, nil
			m, control := liveManager(t)
			s.LiveCapture = m
			input := liveInput()
			require.NoError(t, s.SendIterableSeries(input))
			require.Equal(t, len(input.values)+1, input.calls)
			require.ElementsMatch(t, wire, f.wire)
			batch, err := m.Read(telemetrycapture.ReadRequest{Control: control})
			require.NoError(t, err)
			defer batch.Release()
			require.Len(t, batch.Records, 1)
			record := batch.Records[0]
			require.Len(t, record.Payload.Series, 3, "oversized and unselected items are not outgoing evidence")
			require.Equal(t, uint32(123), record.Payload.Series[0].Source)
			require.Equal(t, 1234567890.125, record.Payload.Series[0].Points[0].Timestamp)
			require.Contains(t, record.Payload.Series[0].Tags, "device:native-device", "copy must precede mutation")
			require.Empty(t, record.Payload.Series[0].Device)
			require.Equal(t, "percent", record.Payload.Series[0].Unit)
			require.Equal(t, "System", record.Payload.Series[0].SourceTypeName)
			require.Equal(t, []telemetrycapture.Resource{{Type: "device", Name: "native-resource"}}, record.Payload.Series[0].Resources)
			input.values[0].Resources[0].Name = "changed"
			require.Equal(t, "native-resource", record.Payload.Series[0].Resources[0].Name)
			input.values[0].Points[0].Value = -1
			require.Equal(t, 7.5, record.Payload.Series[0].Points[0].Value)
		})
	}
}

func TestLiveNetworkMetricCapturePreservesRatesAndDelivery(t *testing.T) {
	names := []string{
		"system.net.bytes_rcvd", "system.net.bytes_sent",
		"system.net.packets_in.count", "system.net.packets_out.count",
		"system.net.tcp.retrans_packs", "system.net.tcp.sent_packs", "system.net.tcp.rcv_packs",
	}
	input := func() *liveTestSource {
		source := &liveTestSource{index: -1}
		for i, name := range names {
			source.values = append(source.values, &metrics.Serie{
				Name: name, Host: "native-host", Source: metrics.MetricSource(123),
				MType: metrics.APIRateType, Interval: 15,
				Tags:   tagset.NewCompositeTags([]string{"device:en0"}, nil),
				Points: []metrics.Point{{Ts: 1234567890.125, Value: float64(i) + 0.25}},
			})
		}
		return source
	}
	for _, version := range []int{1, 2, 3} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			s, f := liveSerializer(t, version)
			require.NoError(t, s.SendIterableSeries(input()))
			baseline := slices.Clone(f.wire)
			f.wire, f.payloads = nil, nil
			manager, control := liveManager(t)
			s.LiveCapture = manager
			source := input()
			require.NoError(t, s.SendIterableSeries(source))
			require.Equal(t, len(names)+1, source.calls)
			require.ElementsMatch(t, baseline, f.wire)
			batch, err := manager.Read(telemetrycapture.ReadRequest{Control: control})
			require.NoError(t, err)
			defer batch.Release()
			require.Len(t, batch.Records, 1)
			require.Len(t, batch.Records[0].Payload.Series, len(names))
			for i, series := range batch.Records[0].Payload.Series {
				require.Equal(t, names[i], series.Name)
				require.Equal(t, int32(metrics.APIRateType), series.Type)
				require.Equal(t, int64(15), series.Interval)
				require.Equal(t, uint32(123), series.Source)
				require.Equal(t, []string{"device:en0"}, series.Tags)
				require.Equal(t, []telemetrycapture.Point{{Timestamp: 1234567890.125, Value: float64(i) + 0.25}}, series.Points)
				source.values[i].Points[0].Value = -1
				require.Equal(t, float64(i)+0.25, series.Points[0].Value)
			}
		})
	}
}

func TestLiveMetricCaptureOverflowPreservesProductionError(t *testing.T) {
	s, f := liveSerializer(t, 2)
	m, _ := liveManager(t)
	s.LiveCapture = m
	reservations := make([]*telemetrycapture.Reservation, telemetrycapture.MaxRecords)
	for i := range reservations {
		reservations[i] = m.Begin(telemetrycapture.Metrics, time.Now(), time.Second, 1)
	}
	defer func() {
		for _, r := range reservations {
			r.Discard()
		}
	}()
	expected := errors.New("production enqueue error")
	f.err = expected
	require.ErrorIs(t, s.SendIterableSeries(liveInput()), expected)
	require.NotEmpty(t, f.wire)
	require.Equal(t, telemetrycapture.Failed, m.Status().State)
}

func TestLiveMetricCaptureConcurrentFlushes(t *testing.T) {
	s, _ := liveSerializer(t, 2)
	m, control := liveManager(t)
	s.LiveCapture = m
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Go(func() { require.NoError(t, s.SendIterableSeries(liveInput())) })
	}
	workers.Wait()
	batch, err := m.Read(telemetrycapture.ReadRequest{Control: control})
	require.NoError(t, err)
	defer batch.Release()
	require.Len(t, batch.Records, 8)
	cycles := map[uint64]bool{}
	for _, record := range batch.Records {
		require.False(t, cycles[record.CycleID])
		cycles[record.CycleID] = true
		require.Len(t, record.Payload.Series, 3)
	}
}

type unavailableStreamCompressor struct{ compression.Compressor }

func (unavailableStreamCompressor) NewStreamCompressor(*bytes.Buffer) compression.StreamCompressor {
	return nil
}

func TestLiveMetricCaptureEarlySerializerFailureDoesNotConsumeAgain(t *testing.T) {
	s, _ := liveSerializer(t, 2)
	// An unavailable stream compressor causes normal serialization to fail before
	// iteration. Capture must preserve that behavior and fail its own session.
	s.Strategy = unavailableStreamCompressor{s.Strategy}
	m, _ := liveManager(t)
	s.LiveCapture = m
	input := liveInput()
	require.Error(t, s.SendIterableSeries(input))
	require.Zero(t, input.calls)
	require.Equal(t, telemetrycapture.Failed, m.Status().State)
}

func TestLiveMetricCaptureEarlyItemFailureDoesNotDrainSource(t *testing.T) {
	s, _ := liveSerializer(t, 1)
	s.RequireCompleteDelivery = true
	m, _ := liveManager(t)
	s.LiveCapture = m
	input := liveInput()
	input.values[1] = nil
	require.Error(t, s.SendIterableSeries(input))
	require.Equal(t, 2, input.calls, "capture must not consume beyond normal serialization")
	require.Equal(t, telemetrycapture.Failed, m.Status().State)
}

func TestLiveMetricCaptureV1ExcludesNoIndex(t *testing.T) {
	s, _ := liveSerializer(t, 1)
	m, control := liveManager(t)
	s.LiveCapture = m
	input := liveInput()
	input.values[0].NoIndex = true
	require.NoError(t, s.SendIterableSeries(input))
	batch, err := m.Read(telemetrycapture.ReadRequest{Control: control})
	require.NoError(t, err)
	defer batch.Release()
	require.Len(t, batch.Records, 1)
	require.Len(t, batch.Records[0].Payload.Series, 2)
	for _, series := range batch.Records[0].Payload.Series {
		require.NotEqual(t, "system.cpu.user", series.Name)
	}
}

func TestLiveMetricCapturePipelineFiltersAndDeduplicates(t *testing.T) {
	for _, version := range []int{2, 3} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			s, f := liveSerializer(t, version)
			m, control := liveManager(t)
			input := liveInput()
			capture := beginLiveSeries(m, 15*time.Second, input)
			allowlist := map[string]struct{}{"system.mem.total": {}}
			endpoint := endpoints.SeriesEndpoint
			if version == 3 {
				endpoint = endpoints.V3SeriesEndpoint
			}
			pipelines := metricsserializer.PipelineSet{}
			pipelines.Add(metricsserializer.PipelineConfig{Filter: metricsserializer.NewMapFilter(allowlist), V3: version == 3}, metricsserializer.PipelineDestination{Resolver: f.GetDomainResolvers()[0], Endpoint: endpoint})
			// A distinct filter creates a second encoding pipeline accepting the
			// same source item. Capture must preserve its single semantic copy.
			pipelines.Add(metricsserializer.PipelineConfig{Filter: metricsserializer.NewMapFilter(allowlist), V3: version == 3}, metricsserializer.PipelineDestination{Resolver: f.GetDomainResolvers()[0], Endpoint: endpoint})
			require.Len(t, pipelines, 2)
			serializer := metricsserializer.CreateIterableSeries(capture)
			err := serializer.MarshalSplitCompressPipelines(s.config, s.Strategy, pipelines)
			require.NoError(t, err)
			err = pipelines.Send(f, s.protobufExtraHeadersWithCompression)
			capture.finish(err)
			require.NoError(t, err)
			require.Equal(t, len(input.values)+1, input.calls)
			batch, err := m.Read(telemetrycapture.ReadRequest{Control: control})
			require.NoError(t, err)
			defer batch.Release()
			require.Len(t, batch.Records, 1)
			require.Len(t, batch.Records[0].Payload.Series, 1)
			require.Equal(t, "system.mem.total", batch.Records[0].Payload.Series[0].Name)
		})
	}
}

func TestLiveMetricCapturePreservesNoIndexInSupportingProtocols(t *testing.T) {
	for _, version := range []int{2, 3} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			s, _ := liveSerializer(t, version)
			m, control := liveManager(t)
			s.LiveCapture = m
			input := liveInput()
			input.values[0].NoIndex = true
			require.NoError(t, s.SendIterableSeries(input))
			batch, err := m.Read(telemetrycapture.ReadRequest{Control: control})
			require.NoError(t, err)
			defer batch.Release()
			require.Len(t, batch.Records, 1)
			require.True(t, batch.Records[0].Payload.Series[0].NoIndex)
		})
	}
}
