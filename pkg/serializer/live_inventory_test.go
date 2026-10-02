// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && zlib && zstd

package serializer

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/stretchr/testify/require"
)

type liveInventoryInput struct {
	stream    telemetrycapture.Stream
	at        time.Time
	cadence   time.Duration
	copies    int
	panicCopy bool
	oversize  bool
	ambiguous bool
}

func (*liveInventoryInput) MarshalJSON() ([]byte, error) {
	return []byte(`{"hostname":"native-host","agent_metadata":{"api_key":"secret-sentinel","full_configuration":"secret-sentinel","agent_version":"7.85.0"}}`), nil
}
func (p *liveInventoryInput) CaptureInventoryStream() telemetrycapture.Stream { return p.stream }
func (p *liveInventoryInput) CaptureInventorySchedule() (time.Time, time.Duration) {
	return p.at, p.cadence
}
func (p *liveInventoryInput) CaptureInventorySize() int64 {
	if p.oversize {
		return telemetrycapture.MaxItemBytes + 1
	}
	return 4096
}
func (p *liveInventoryInput) CopyCaptureInventory() *telemetrycapture.Inventory {
	p.copies++
	if p.panicCopy {
		panic("private projection failure")
	}
	out := &telemetrycapture.Inventory{Hostname: "native-host", UUID: "native-uuid", Timestamp: p.at.UnixNano()}
	switch p.stream {
	case telemetrycapture.AgentInventory:
		out.Agent = &telemetrycapture.AgentInventoryMetadata{AgentVersion: "7.85.0", InfrastructureMode: "end_user_device"}
	case telemetrycapture.HostInventory:
		out.Host = &telemetrycapture.HostInventoryMetadata{OS: "Darwin", MemoryTotalKb: 123456}
	case telemetrycapture.HostSystemInfo:
		out.SystemInfo = &telemetrycapture.HostSystemInfoMetadata{Manufacturer: "Example", SerialNumber: "native-serial"}
	}
	if p.ambiguous {
		out.Agent, out.Host, out.SystemInfo = &telemetrycapture.AgentInventoryMetadata{}, &telemetrycapture.HostInventoryMetadata{}, &telemetrycapture.HostSystemInfoMetadata{}
	}
	return out
}

func (f *liveTestForwarder) SubmitMetadata(payloads transaction.BytesPayloads, headers http.Header) error {
	for _, p := range payloads {
		if err := f.record(p, endpoints.V1MetadataEndpoint.Route, "inventory-v1", "primary", headers); err != nil {
			return err
		}
	}
	return nil
}

func inventoryManager(t *testing.T, stream telemetrycapture.Stream) (*telemetrycapture.Manager, telemetrycapture.Control) {
	t.Helper()
	m := telemetrycapture.NewManager("core-agent", "fixture", "fixture")
	t.Cleanup(m.Close)
	require.NoError(t, m.Register(telemetrycapture.Capability{Stream: stream, Cadence: 10 * time.Minute}))
	c := telemetrycapture.Control{ProtocolVersion: 1, SessionID: "inventory-test-session"}
	_, err := m.Prepare(telemetrycapture.PrepareRequest{Control: c, Streams: []telemetrycapture.Stream{stream}})
	require.NoError(t, err)
	_, err = m.Activate(c)
	require.NoError(t, err)
	return m, c
}

func TestLiveInventoryPreservesDeliveryAndCapturesOnlyOwnedProjection(t *testing.T) {
	for _, stream := range []telemetrycapture.Stream{telemetrycapture.AgentInventory, telemetrycapture.HostInventory, telemetrycapture.HostSystemInfo} {
		t.Run(string(stream), func(t *testing.T) {
			s, f := liveSerializer(t, 2)
			p := &liveInventoryInput{stream: stream, at: time.Now(), cadence: 10 * time.Minute}
			require.NoError(t, s.SendMetadata(p))
			require.Zero(t, p.copies)
			baseline := slices.Clone(f.wire)
			f.wire, f.payloads = nil, nil
			m, c := inventoryManager(t, stream)
			s.LiveCapture = m
			require.NoError(t, s.SendMetadata(p))
			require.Equal(t, baseline, f.wire)
			require.Equal(t, 1, p.copies)
			require.Nil(t, f.payloads[0].Capture())
			batch, err := m.Read(telemetrycapture.ReadRequest{Control: c})
			require.NoError(t, err)
			defer batch.Release()
			require.Len(t, batch.Records, 1)
			r := batch.Records[0]
			require.Equal(t, p.at, r.CollectedAt)
			require.Equal(t, p.cadence, r.Cadence)
			require.Equal(t, "inventory-v1", r.Payload.Routes[0].Protocol)
			require.Equal(t, "/api/v1/metadata", r.Payload.Routes[0].Endpoint)
			encoded, err := json.Marshal(r)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), "secret-sentinel")
			// Unrelated inventory submissions are neither captured nor errors.
			require.NoError(t, s.SendMetadata(unsupportedMetadata{p}))
			require.Equal(t, uint64(1), m.Status().FinalSequence)
			require.Equal(t, telemetrycapture.Active, m.Status().State)
		})
	}
}

func TestLiveInventoryCaptureFailuresPreserveSubmission(t *testing.T) {
	for _, stream := range []telemetrycapture.Stream{telemetrycapture.AgentInventory, telemetrycapture.HostSystemInfo} {
		for _, mode := range []string{"oversize", "panic", "missing-route", "invalid-schedule", "submission-error", "ambiguous"} {
			t.Run(string(stream)+"/"+mode, func(t *testing.T) {
				s, f := liveSerializer(t, 2)
				m, _ := inventoryManager(t, stream)
				s.LiveCapture = m
				p := &liveInventoryInput{stream: stream, at: time.Now(), cadence: time.Minute}
				switch mode {
				case "oversize":
					p.oversize = true
				case "panic":
					p.panicCopy = true
				case "missing-route":
					f.omitRoute = true
				case "invalid-schedule":
					p.at = time.Time{}
				case "submission-error":
					f.err = errors.New("normal error")
				case "ambiguous":
					p.ambiguous = true
				}
				err := s.SendMetadata(p)
				if f.err != nil {
					require.ErrorIs(t, err, f.err)
				} else {
					require.NoError(t, err)
				}
				require.Len(t, f.wire, 1)
				require.Nil(t, f.payloads[0].Capture())
				require.Equal(t, telemetrycapture.Failed, m.Status().State)
				if mode == "oversize" {
					require.Zero(t, p.copies)
				}
			})
		}
	}
}
