// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubeactionsimpl

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type capturedEvent struct {
	eventType string
	payload   []byte
}

// fakeForwarder captures the events sent through SendEventPlatformEventBlocking.
type fakeForwarder struct {
	events []capturedEvent
}

func (f *fakeForwarder) SendEventPlatformEvent(*message.Message, string) error { return nil }

func (f *fakeForwarder) SendEventPlatformEventBlocking(m *message.Message, eventType string) error {
	f.events = append(f.events, capturedEvent{eventType: eventType, payload: m.GetContent()})
	return nil
}

func (f *fakeForwarder) Purge() map[string][]*message.Message { return nil }

func sampleReport() kubeactions.ActionReport {
	return kubeactions.ActionReport{
		ActionID:          "action-1",
		ActionType:        kubeactions.ActionTypeDeletePod,
		OrgID:             42,
		RequestedBy:       "alice",
		ResourceID:        "pod-uid",
		ResourceKind:      "Pod",
		ResourceName:      "my-pod",
		ResourceNamespace: "default",
	}
}

func decodeEvent(t *testing.T, payload []byte) ActionResultEvent {
	t.Helper()
	var ev ActionResultEvent
	require.NoError(t, json.Unmarshal(payload, &ev))
	return ev
}

func TestResultReporter_ReportReceived(t *testing.T) {
	fwd := &fakeForwarder{}
	r := newResultReporter(fwd, "my-cluster", "cluster-id")

	r.ReportReceived(sampleReport())

	require.Len(t, fwd.events, 1)
	assert.Equal(t, eventplatform.EventTypeKubeActions, fwd.events[0].eventType)

	ev := decodeEvent(t, fwd.events[0].payload)
	assert.Equal(t, kubeactions.EventTypeActionReceived, ev.EventType)
	assert.Equal(t, kubeactions.StatusSuccess, ev.Status)
	assert.Equal(t, "action-1", ev.ActionID)
	assert.Equal(t, int64(42), ev.OrgID)
	assert.Equal(t, "alice", ev.RequestedBy)
	assert.Equal(t, "my-cluster", ev.ClusterName)
	assert.Equal(t, "cluster-id", ev.ClusterID)
	assert.Equal(t, "Pod", ev.ResourceKind)
	assert.Equal(t, "default", ev.ResourceNamespace)
}

func TestResultReporter_ReportProgress(t *testing.T) {
	fwd := &fakeForwarder{}
	r := newResultReporter(fwd, "my-cluster", "cluster-id")

	r.ReportProgress(sampleReport(), "halfway there")

	require.Len(t, fwd.events, 1)
	ev := decodeEvent(t, fwd.events[0].payload)
	assert.Equal(t, kubeactions.EventTypeActionProgress, ev.EventType)
	assert.Equal(t, "in_progress", ev.Status)
	assert.Equal(t, "halfway there", ev.Message)
}

func TestResultReporter_ReportResult(t *testing.T) {
	fwd := &fakeForwarder{}
	r := newResultReporter(fwd, "my-cluster", "cluster-id")

	r.ReportResult(sampleReport(), kubeactions.ExecutionResult{
		Status:   kubeactions.StatusFailed,
		Message:  "boom",
		Payloads: map[string][]byte{"configmaps/default/x": []byte(`{"a":1}`)},
	})

	require.Len(t, fwd.events, 1)
	ev := decodeEvent(t, fwd.events[0].payload)
	assert.Equal(t, kubeactions.EventTypeActionExecuted, ev.EventType)
	assert.Equal(t, kubeactions.StatusFailed, ev.Status)
	assert.Equal(t, "boom", ev.Message)
	assert.Contains(t, ev.Payloads, "configmaps/default/x")
}

func TestResultReporter_NilForwarderIsSafe(t *testing.T) {
	r := newResultReporter(nil, "my-cluster", "cluster-id")
	assert.NotPanics(t, func() {
		r.ReportReceived(sampleReport())
		r.ReportProgress(sampleReport(), "msg")
		r.ReportResult(sampleReport(), kubeactions.ExecutionResult{Status: kubeactions.StatusSuccess})
	})
}
