// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package eventplatformimpl

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformreceiver "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

type captureTestReceiver struct {
	eventplatformreceiver.Component
	calls int
}

func (r *captureTestReceiver) HandleMessage(_ *message.Message, _ []byte, _ string) { r.calls++ }

func softwareCaptureForwarder(t *testing.T) (*defaultEventPlatformForwarder, *telemetrycapture.Manager, telemetrycapture.Control, *captureTestReceiver) {
	t.Helper()
	m := telemetrycapture.NewManager("core-agent", "fixture", "fixture")
	t.Cleanup(m.Close)
	require.NoError(t, m.Register(telemetrycapture.Capability{Stream: telemetrycapture.Software, Cadence: 17 * time.Minute}))
	control := telemetrycapture.Control{ProtocolVersion: telemetrycapture.ProtocolVersion, SessionID: "software-capture-session"}
	_, err := m.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{telemetrycapture.Software}})
	require.NoError(t, err)
	_, err = m.Activate(control)
	require.NoError(t, err)
	receiver := &captureTestReceiver{}
	f := &defaultEventPlatformForwarder{captureManager: m, pipelines: map[string]*passthroughPipeline{
		eventplatform.EventTypeSoftwareInventory: {in: make(chan *message.Message, 1), eventPlatformReceiver: receiver},
		eventplatform.EventTypeNetworkPath:       {in: make(chan *message.Message, 1), eventPlatformReceiver: receiver},
	}}
	f.SetSoftwareCaptureCadence(17 * time.Minute)
	return f, m, control, receiver
}

func TestSoftwareObservationCopiesCompleteMessageWithoutChangingDelivery(t *testing.T) {
	f, m, control, receiver := softwareCaptureForwarder(t)
	body := []byte(`{"hostname":"native-host","metadata":{"software":[{"name":"first"},{"name":"second"}]}}`)
	timestamp := time.Now().UnixNano()
	msg := message.NewMessage(body, nil, "", timestamp)
	require.NoError(t, f.SendEventPlatformEvent(msg, eventplatform.EventTypeSoftwareInventory))
	require.Same(t, msg, <-f.pipelines[eventplatform.EventTypeSoftwareInventory].in)
	require.Equal(t, body, msg.GetContent())
	require.Equal(t, 1, receiver.calls, "capture must not replace or enable diagnostics")
	batch, err := m.Read(telemetrycapture.ReadRequest{Control: control})
	require.NoError(t, err)
	require.Len(t, batch.Records, 1)
	record := batch.Records[0]
	require.Equal(t, 17*time.Minute, record.Cadence)
	require.Equal(t, timestamp, record.Payload.Software.Timestamp)
	require.Equal(t, timestamp, record.CollectedAt.UnixNano())
	require.Equal(t, body, record.Payload.Software.Body)
	body[0] = '!'
	require.Equal(t, byte('{'), record.Payload.Software.Body[0], "capture must own its copy")
	batch.Release()
	_, err = m.Stop(context.Background(), control)
	require.NoError(t, err)
	batch, err = m.Read(telemetrycapture.ReadRequest{Control: control, Cursor: 1})
	require.NoError(t, err)
	batch.Release()
	require.Equal(t, telemetrycapture.Stopped, m.Status().State)
}

func TestSoftwareObservationIgnoresOtherEventsBeforeReadingMessage(t *testing.T) {
	f, m, control, receiver := softwareCaptureForwarder(t)
	// nil deliberately makes timestamp/content access observable as a panic.
	require.NoError(t, f.SendEventPlatformEvent(nil, eventplatform.EventTypeNetworkPath))
	require.Nil(t, <-f.pipelines[eventplatform.EventTypeNetworkPath].in)
	require.Equal(t, 1, receiver.calls)
	batch, err := m.Read(telemetrycapture.ReadRequest{Control: control})
	require.NoError(t, err)
	defer batch.Release()
	require.Empty(t, batch.Records)
	require.Equal(t, telemetrycapture.Active, batch.Status.State)
}

func TestSoftwareObservationOverflowDoesNotBlockSubmission(t *testing.T) {
	f, m, _, _ := softwareCaptureForwarder(t)
	for range telemetrycapture.MaxRecords {
		r := m.Begin(telemetrycapture.Software, time.Now(), time.Minute, 1)
		require.NotNil(t, r)
		defer r.Discard()
	}
	msg := message.NewMessage([]byte("complete snapshot"), nil, "", time.Now().UnixNano())
	done := make(chan error, 1)
	go func() { done <- f.SendEventPlatformEvent(msg, eventplatform.EventTypeSoftwareInventory) }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("capture overflow blocked production submission")
	}
	require.Same(t, msg, <-f.pipelines[eventplatform.EventTypeSoftwareInventory].in)
	require.Equal(t, telemetrycapture.Failed, m.Status().State)
}

func TestSoftwareObservationPreservesFullProductionQueueError(t *testing.T) {
	f, m, _, _ := softwareCaptureForwarder(t)
	msg := message.NewMessage([]byte("complete snapshot"), nil, "", time.Now().UnixNano())
	p := f.pipelines[eventplatform.EventTypeSoftwareInventory]
	p.in <- msg
	f.captureManager = nil
	baseline := f.SendEventPlatformEvent(msg, eventplatform.EventTypeSoftwareInventory)
	f.captureManager = m
	captured := f.SendEventPlatformEvent(msg, eventplatform.EventTypeSoftwareInventory)
	require.EqualError(t, captured, baseline.Error())
	require.Equal(t, 1, len(p.in))
	require.Same(t, msg, <-p.in)
	require.Equal(t, telemetrycapture.Failed, m.Status().State)
	require.Zero(t, m.Status().FinalSequence, "rejected snapshots must not count as observed output")
}

func TestSoftwareObservationFailurePreservesOriginalMessage(t *testing.T) {
	f, m, _, _ := softwareCaptureForwarder(t)
	// Invalid capture input invalidates its session, while the original event
	// follows the same production path and preserves the original return value.
	msg := message.NewMessage([]byte("complete snapshot"), nil, "", 0)
	require.NoError(t, f.SendEventPlatformEvent(msg, eventplatform.EventTypeSoftwareInventory))
	require.Same(t, msg, <-f.pipelines[eventplatform.EventTypeSoftwareInventory].in)
	require.Equal(t, telemetrycapture.Failed, m.Status().State)
}
