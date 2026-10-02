// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux || (windows && npm)

package sender

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
	"github.com/stretchr/testify/require"
)

type captureForwarder struct{ sent chan payload }

func (f *captureForwarder) SubmitConnectionChecks(p transaction.BytesPayloads, h http.Header) (chan forwarder.Response, error) {
	for _, body := range p {
		f.sent <- payload{body: body.GetContent(), headers: h}
	}
	responses := make(chan forwarder.Response)
	close(responses)
	return responses, nil
}

func TestDirectSenderObservesGroupsWithoutChangingForwarding(t *testing.T) {
	for _, mode := range []string{"disabled", "active", "overflow", "stopped"} {
		t.Run(mode, func(t *testing.T) {
			manager := telemetrycapture.NewManager("system-probe", "fixture", "fixture")
			defer manager.Close()
			require.NoError(t, manager.Register(telemetrycapture.Capability{Stream: telemetrycapture.Connections, Cadence: 30 * time.Second, ConnectionOwner: "direct"}))
			control := telemetrycapture.Control{ProtocolVersion: telemetrycapture.ProtocolVersion, SessionID: "direct-sender-session"}
			if mode != "disabled" {
				_, err := manager.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Connections}})
				require.NoError(t, err)
				_, err = manager.Activate(control)
				require.NoError(t, err)
			}
			if mode == "overflow" {
				for range telemetrycapture.MaxRecords {
					reservation := manager.Begin(telemetrycapture.Connections, time.Now(), time.Second, 1024)
					require.NotNil(t, reservation)
					require.True(t, reservation.Commit(telemetrycapture.Payload{Chunks: []telemetrycapture.Chunk{{Body: []byte{1}}}}))
				}
			}
			if mode == "stopped" {
				_, err := manager.Stop(context.Background(), control)
				require.NoError(t, err)
			}
			at := time.Unix(1700000000, 987654321)
			group := result{collectedAt: at, expectedChunks: 3}
			for i := 0; i < 3; i++ {
				body, err := api.EncodePayload(&model.CollectorConnections{HostName: "native-host", GroupId: 17, GroupSize: 3, Connections: []*model.Connection{{Pid: int32(i + 1)}}})
				require.NoError(t, err)
				h := make(http.Header)
				h.Set(headers.HostHeader, "native-host")
				h.Set(headers.TimestampHeader, strconv.FormatInt(at.Unix(), 10))
				h.Set(headers.RequestIDHeader, strconv.Itoa((41<<14)+i))
				group.payloads = append(group.payloads, payload{body: body, headers: h})
				group.size += int64(len(body))
			}
			f := &captureForwarder{sent: make(chan payload, 3)}
			d := &directSender{captureManager: manager, checkInterval: 30 * time.Second, forwarder: f, log: logmock.New(t), resultsQueue: api.NewWeightedQueue(10, 1<<20)}
			d.resultsQueue.Add(group)
			done := make(chan struct{})
			go func() { defer close(done); d.submitLoop() }()
			for _, expected := range group.payloads {
				select {
				case sent := <-f.sent:
					require.Equal(t, expected.body, sent.body)
					require.Equal(t, expected.headers, sent.headers)
				case <-time.After(5 * time.Second):
					t.Fatal("capture blocked production forwarding")
				}
			}
			d.resultsQueue.Stop()
			<-done
			switch mode {
			case "active":
				batch, err := manager.Read(telemetrycapture.ReadRequest{Control: control})
				require.NoError(t, err)
				defer batch.Release()
				require.Len(t, batch.Records, 1)
				record := batch.Records[0]
				require.Equal(t, at, record.CollectedAt)
				require.Len(t, record.Payload.Chunks, 3)
				messages, err := api.DecodeCaptureGroup(record)
				require.NoError(t, err)
				for i, m := range messages {
					require.Equal(t, int32(i+1), m.(*model.CollectorConnections).Connections[0].Pid)
				}
				group.payloads[0].body[0] ^= 0xff
				require.NotEqual(t, group.payloads[0].body, record.Payload.Chunks[0].Body)
			case "overflow":
				require.Equal(t, telemetrycapture.Failed, manager.Status().State)
			case "stopped":
				require.Equal(t, telemetrycapture.Stopped, manager.Status().State)
				require.Zero(t, manager.Status().FinalSequence)
			case "disabled":
				require.Zero(t, manager.Status().FinalSequence)
			}
		})
	}
}

func TestDirectSenderEmptyCollectionAndEncodingLoss(t *testing.T) {
	manager := telemetrycapture.NewManager("system-probe", "fixture", "fixture")
	defer manager.Close()
	require.NoError(t, manager.Register(telemetrycapture.Capability{Stream: telemetrycapture.Connections, Cadence: time.Second, ConnectionOwner: "direct"}))
	control := telemetrycapture.Control{ProtocolVersion: telemetrycapture.ProtocolVersion, SessionID: "empty-direct-connection-session"}
	_, err := manager.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Connections}})
	require.NoError(t, err)
	_, err = manager.Activate(control)
	require.NoError(t, err)
	d := &directSender{captureManager: manager, checkInterval: time.Second}
	d.observeCaptureGroup(result{collectedAt: time.Now()})
	require.Equal(t, telemetrycapture.Active, manager.Status().State)
	require.Zero(t, manager.Status().FinalSequence)
	d.observeCaptureGroup(result{collectedAt: time.Now(), expectedChunks: 1})
	require.Equal(t, telemetrycapture.Failed, manager.Status().State)
}
