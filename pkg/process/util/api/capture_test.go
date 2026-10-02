// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/stretchr/testify/require"
)

func captureGroupManager(t *testing.T, stream telemetrycapture.Stream) (*telemetrycapture.Manager, telemetrycapture.Control) {
	t.Helper()
	m := telemetrycapture.NewManager("fixture-producer", "fixture", "fixture")
	t.Cleanup(m.Close)
	require.NoError(t, m.Register(telemetrycapture.Capability{Stream: stream, Cadence: time.Second}))
	c := telemetrycapture.Control{ProtocolVersion: 1, SessionID: "complete-groups-session"}
	_, err := m.Prepare(telemetrycapture.PrepareRequest{Control: c, Streams: []telemetrycapture.Stream{stream}})
	require.NoError(t, err)
	_, err = m.Activate(c)
	require.NoError(t, err)
	return m, c
}

func captureGroupInput(t *testing.T, stream telemetrycapture.Stream, count int) ([][]byte, []http.Header) {
	t.Helper()
	bodies := make([][]byte, count)
	head := make([]http.Header, count)
	for i := range bodies {
		var body model.MessageBody = &model.CollectorProc{HostName: "native-host", GroupId: 123, GroupSize: int32(count)}
		if stream == telemetrycapture.Connections {
			body = &model.CollectorConnections{HostName: "native-host", GroupId: 123, GroupSize: int32(count)}
		}
		var err error
		bodies[i], err = EncodePayload(body)
		require.NoError(t, err)
		head[i] = make(http.Header)
		head[i].Set(headers.HostHeader, "native-host")
		head[i].Set(headers.RequestIDHeader, strconv.Itoa((27<<14)+i))
		head[i].Set("Authorization", "credential-sentinel")
		head[i].Set("DD-API-KEY", "credential-sentinel")
	}
	return bodies, head
}

func TestCaptureCompleteGroupsOwnOrderedChunks(t *testing.T) {
	for _, stream := range []telemetrycapture.Stream{telemetrycapture.Processes, telemetrycapture.Connections} {
		t.Run(string(stream), func(t *testing.T) {
			m, c := captureGroupManager(t, stream)
			bodies, head := captureGroupInput(t, stream, 3)
			at := time.Unix(1700000000, 123456789)
			ObserveCaptureGroup(m, stream, at, 17*time.Second, len(bodies), func(i int) ([]byte, http.Header) { return bodies[i], head[i] })
			batch, err := m.Read(telemetrycapture.ReadRequest{Control: c})
			require.NoError(t, err)
			require.Len(t, batch.Records, 1)
			record := batch.Records[0]
			require.Equal(t, at, record.CollectedAt)
			require.Equal(t, 17*time.Second, record.Cadence)
			messages, err := DecodeCaptureGroup(record)
			require.NoError(t, err)
			require.Len(t, messages, 3)
			for i, chunk := range record.Payload.Chunks {
				require.Equal(t, bodies[i], chunk.Body)
				require.NotContains(t, chunk.Headers, "Authorization")
				require.NotContains(t, chunk.Headers, "DD-API-KEY")
				bodies[i][0] ^= 0xff
				head[i].Set(headers.HostHeader, "mutated")
				require.NotEqual(t, bodies[i], chunk.Body)
				require.Equal(t, "native-host", chunk.Headers[headers.HostHeader])
			}
			batch.Release()
		})
	}
}

func TestDecodeCaptureRejectsIncompleteOrUnorderedGroups(t *testing.T) {
	for _, mode := range []string{"partial", "reordered", "mixed", "invalid", "wrong-stream", "request-prefix"} {
		t.Run(mode, func(t *testing.T) {
			m, c := captureGroupManager(t, telemetrycapture.Processes)
			bodies, head := captureGroupInput(t, telemetrycapture.Processes, 3)
			switch mode {
			case "partial":
				bodies = bodies[:2]
			case "reordered":
				head[0], head[1] = head[1], head[0]
			case "mixed":
				bodies[1], _ = EncodePayload(&model.CollectorProc{HostName: "native-host", GroupId: 124, GroupSize: 3})
			case "invalid":
				bodies[1] = []byte{1, 2, 3}
			case "wrong-stream":
				bodies[1], _ = EncodePayload(&model.CollectorConnections{HostName: "native-host", GroupId: 123, GroupSize: 3})
			case "request-prefix":
				head[1].Set(headers.RequestIDHeader, strconv.Itoa((28<<14)+1))
			}
			ObserveCaptureGroup(m, telemetrycapture.Processes, time.Now(), time.Second, len(bodies), func(i int) ([]byte, http.Header) { return bodies[i], head[i] })
			batch, err := m.Read(telemetrycapture.ReadRequest{Control: c})
			require.NoError(t, err)
			defer batch.Release()
			require.Len(t, batch.Records, 1)
			// Decoding is deliberately a coordinator operation. It must fail the
			// session rather than accepting a partial collection cycle.
			_, err = DecodeCaptureGroup(batch.Records[0])
			require.Error(t, err)
			require.NoError(t, m.Fail(c))
			require.Equal(t, telemetrycapture.Failed, m.Status().State)
		})
	}
}

func TestCaptureGroupsDisabledAndFailureIsolation(t *testing.T) {
	var dormant *telemetrycapture.Manager
	ObserveCaptureGroup(dormant, telemetrycapture.Processes, time.Now(), time.Second, 1, func(int) ([]byte, http.Header) { panic("must not inspect disabled input") })
	for _, mode := range []string{"overflow", "panic", "empty"} {
		t.Run(mode, func(t *testing.T) {
			m, _ := captureGroupManager(t, telemetrycapture.Processes)
			if mode == "overflow" {
				for range telemetrycapture.MaxRecords {
					r := m.Begin(telemetrycapture.Processes, time.Now(), time.Second, 1024)
					require.NotNil(t, r)
					require.True(t, r.Commit(telemetrycapture.Payload{Chunks: []telemetrycapture.Chunk{{Body: []byte{1}}}}))
				}
			}
			require.NotPanics(t, func() {
				ObserveCaptureGroup(m, telemetrycapture.Processes, time.Now(), time.Second, 1, func(int) ([]byte, http.Header) {
					if mode == "panic" {
						panic("producer callback failure")
					}
					return nil, nil
				})
			})
			require.Equal(t, telemetrycapture.Failed, m.Status().State)
		})
	}
}

func TestCaptureGroupStopWaitsForReservedCopy(t *testing.T) {
	m, c := captureGroupManager(t, telemetrycapture.Connections)
	bodies, head := captureGroupInput(t, telemetrycapture.Connections, 2)
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		ObserveCaptureGroup(m, telemetrycapture.Connections, time.Now(), time.Second, 2, func(i int) ([]byte, http.Header) {
			if i == 0 {
				close(entered)
				<-release
			}
			return bodies[i], head[i]
		})
		close(done)
	}()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = m.Stop(ctx, c)
	require.Equal(t, telemetrycapture.Stopping, m.Status().State)
	close(release)
	<-done
	batch, err := m.Read(telemetrycapture.ReadRequest{Control: c})
	require.NoError(t, err)
	require.Len(t, batch.Records, 1)
	sequence := batch.Records[0].Sequence
	_, err = DecodeCaptureGroup(batch.Records[0])
	require.NoError(t, err)
	batch.Release()
	ack, err := m.Read(telemetrycapture.ReadRequest{Control: c, Cursor: sequence})
	require.NoError(t, err)
	ack.Release()
	status, err := m.Stop(context.Background(), c)
	require.NoError(t, err)
	require.Equal(t, telemetrycapture.Stopped, status.State)
}
