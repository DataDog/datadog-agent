// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test && zlib && zstd

package serializer

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/endpoints"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

type liveMetadataInput struct {
	at        time.Time
	cadence   time.Duration
	panicCopy bool
	copies    int
}

func (*liveMetadataInput) MarshalJSON() ([]byte, error) {
	return []byte(`{"apiKey":"native-credential","internalHostname":"native-host"}`), nil
}
func (p *liveMetadataInput) CaptureMetadataSchedule() (time.Time, time.Duration) {
	return p.at, p.cadence
}
func (*liveMetadataInput) CaptureMetadataSize() int64 { return 4096 }
func (p *liveMetadataInput) CopyCaptureMetadata() *telemetrycapture.HostMetadata {
	p.copies++
	if p.panicCopy {
		panic("projection failure")
	}
	return &telemetrycapture.HostMetadata{Hostname: "native-host", AgentVersion: "producer-version"}
}

type unsupportedMetadata struct{ marshaler.JSONMarshaler }

func (f *liveTestForwarder) SubmitHostMetadata(payloads transaction.BytesPayloads, headers http.Header) error {
	for _, p := range payloads {
		if err := f.record(p, endpoints.HostMetadataEndpoint.Route, "metadata-v2", "primary", headers); err != nil {
			return err
		}
	}
	return nil
}

func metadataManager(t *testing.T) (*telemetrycapture.Manager, telemetrycapture.Control) {
	t.Helper()
	m := telemetrycapture.NewManager("core-agent", "producer-version", "producer-commit")
	t.Cleanup(m.Close)
	require.NoError(t, m.Register(telemetrycapture.Capability{Stream: telemetrycapture.Metadata, Cadence: 30 * time.Minute}))
	c := telemetrycapture.Control{ProtocolVersion: 1, SessionID: "metadata-session-test"}
	_, err := m.Prepare(telemetrycapture.PrepareRequest{Control: c, Streams: []telemetrycapture.Stream{telemetrycapture.Metadata}})
	require.NoError(t, err)
	_, err = m.Activate(c)
	require.NoError(t, err)
	return m, c
}

func TestLiveMetadataDeliveryKeepsCredentialsOutsideCapture(t *testing.T) {
	s, f := liveSerializer(t, 2)
	p := &liveMetadataInput{at: time.Now(), cadence: 17 * time.Minute}
	require.NoError(t, s.SendHostMetadata(p))
	baseline := slices.Clone(f.wire)
	require.Zero(t, p.copies)
	f.wire, f.payloads = nil, nil
	m, c := metadataManager(t)
	s.LiveCapture = m
	require.NoError(t, s.SendHostMetadata(p))
	require.Equal(t, baseline, f.wire)
	require.Equal(t, 1, p.copies)
	raw, err := s.Strategy.Decompress(f.payloads[0].GetContent())
	require.NoError(t, err)
	require.Contains(t, string(raw), "native-credential")
	require.Nil(t, f.payloads[0].Capture())
	batch, err := m.Read(telemetrycapture.ReadRequest{Control: c})
	require.NoError(t, err)
	defer batch.Release()
	require.Len(t, batch.Records, 1)
	record := batch.Records[0]
	require.Equal(t, p.at, record.CollectedAt)
	require.Equal(t, p.cadence, record.Cadence)
	require.Equal(t, "producer-version", record.Payload.Metadata.AgentVersion)
	require.Len(t, record.Payload.Routes, 1)
	require.Equal(t, "metadata-v2", record.Payload.Routes[0].Protocol)
	require.Empty(t, record.Payload.Chunks, "raw metadata transactions must never be retained")
	encoded, err := json.Marshal(record)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "native-credential")
}

func TestLiveMetadataFailureDoesNotChangeDelivery(t *testing.T) {
	for _, mode := range []string{"unsupported", "panic", "missing-route", "overflow", "invalid-schedule"} {
		t.Run(mode, func(t *testing.T) {
			s, f := liveSerializer(t, 2)
			m, _ := metadataManager(t)
			s.LiveCapture = m
			p := &liveMetadataInput{at: time.Now(), cadence: time.Minute}
			var input marshaler.JSONMarshaler = p
			switch mode {
			case "unsupported":
				input = unsupportedMetadata{p}
			case "panic":
				p.panicCopy = true
			case "missing-route":
				f.omitRoute = true
			case "invalid-schedule":
				p.at = time.Time{}
			case "overflow":
				pending := make([]*telemetrycapture.Reservation, telemetrycapture.MaxRecords)
				for i := range pending {
					pending[i] = m.Begin(telemetrycapture.Metadata, time.Now(), time.Minute, 1)
				}
				defer func() {
					for _, r := range pending {
						r.Discard()
					}
				}()
			}
			require.NoError(t, s.SendHostMetadata(input))
			require.Len(t, f.wire, 1)
			require.Equal(t, telemetrycapture.Failed, m.Status().State)
			require.Nil(t, f.payloads[0].Capture())
		})
	}
	t.Run("production error", func(t *testing.T) {
		s, f := liveSerializer(t, 2)
		m, _ := metadataManager(t)
		s.LiveCapture = m
		f.err = errors.New("original submit failure")
		require.ErrorIs(t, s.SendHostMetadata(&liveMetadataInput{at: time.Now(), cadence: time.Minute}), f.err)
		require.Equal(t, telemetrycapture.Failed, m.Status().State)
	})
	t.Run("unselected stream", func(t *testing.T) {
		s, _ := liveSerializer(t, 2)
		m, _ := liveManager(t)
		s.LiveCapture = m
		p := &liveMetadataInput{panicCopy: true}
		require.NoError(t, s.SendHostMetadata(unsupportedMetadata{p}))
		require.Equal(t, telemetrycapture.Active, m.Status().State)
		require.Zero(t, p.copies)
	})
}

type delayedMetadata struct {
	*liveMetadataInput
	entered, released chan struct{}
	panicSchedule     bool
}

func (p *delayedMetadata) CaptureMetadataSchedule() (time.Time, time.Duration) {
	close(p.entered)
	<-p.released
	if p.panicSchedule {
		panic("late old-session failure")
	}
	return p.liveMetadataInput.CaptureMetadataSchedule()
}

func TestLiveMetadataOldProjectionCannotAffectSuccessor(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(strconv.FormatBool(panics), func(t *testing.T) {
			s, f := liveSerializer(t, 2)
			m, old := metadataManager(t)
			s.LiveCapture = m
			p := &delayedMetadata{liveMetadataInput: &liveMetadataInput{at: time.Now(), cadence: time.Minute}, entered: make(chan struct{}), released: make(chan struct{}), panicSchedule: panics}
			result := make(chan error, 1)
			go func() { result <- s.SendHostMetadata(p) }()
			<-p.entered
			stopped, err := m.Stop(context.Background(), old)
			require.NoError(t, err)
			require.Equal(t, telemetrycapture.Stopped, stopped.State)
			next := telemetrycapture.Control{ProtocolVersion: 1, SessionID: "metadata-successor-session"}
			_, err = m.Prepare(telemetrycapture.PrepareRequest{Control: next, Streams: []telemetrycapture.Stream{telemetrycapture.Metadata}})
			require.NoError(t, err)
			_, err = m.Activate(next)
			require.NoError(t, err)
			close(p.released)
			require.NoError(t, <-result)
			require.Len(t, f.wire, 1)
			require.Equal(t, telemetrycapture.Active, m.Status().State)
			require.Zero(t, m.Status().FinalSequence)
			require.Zero(t, p.copies)
		})
	}
}
