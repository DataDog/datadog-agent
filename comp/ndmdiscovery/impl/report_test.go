// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

// fakeSender records every payload handed to the event platform.
type fakeSender struct {
	mu       sync.Mutex
	payloads []metadata.NetworkDevicesMetadata
	types    []string
	err      error
}

func (f *fakeSender) SendEventPlatformEventBlocking(m *message.Message, eventType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	var p metadata.NetworkDevicesMetadata
	if err := json.Unmarshal(m.GetContent(), &p); err != nil {
		return err
	}
	f.payloads = append(f.payloads, p)
	f.types = append(f.types, eventType)
	return nil
}

func (f *fakeSender) recorded() []metadata.NetworkDevicesMetadata {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]metadata.NetworkDevicesMetadata(nil), f.payloads...)
}

func newTestReporter(t *testing.T, sender payloadSender) *payloadReporter {
	t.Helper()
	r := newPayloadReporter(sender, logmock.New(t))
	r.now = func() int64 { return 1700000000 }
	return r
}

func TestReportDevicesSendsOneBatch(t *testing.T) {
	sender := &fakeSender{}
	r := newTestReporter(t, sender)

	devices := []metadata.DiscoveredDeviceMetadata{
		{AutodiscoveryID: "ad-1", RunID: "run-1", IPAddress: "10.0.0.1"},
		{AutodiscoveryID: "ad-1", RunID: "run-1", IPAddress: "10.0.0.2"},
	}
	require.NoError(t, r.ReportDevices("prod", devices))

	got := sender.recorded()
	require.Len(t, got, 1)
	assert.Equal(t, "prod", got[0].Namespace)
	assert.Equal(t, integrations.SNMP, got[0].Integration)
	assert.Equal(t, int64(1700000000), got[0].CollectTimestamp)
	assert.Equal(t, devices, got[0].DiscoveredDevices)
	assert.Equal(t, eventplatform.EventTypeNetworkDevicesMetadata, sender.types[0])
}

func TestReportDevicesSplitsAtBatchSize(t *testing.T) {
	sender := &fakeSender{}
	r := newTestReporter(t, sender)
	var callCount int64
	r.now = func() int64 {
		callCount++
		return 1700000000 + callCount
	}

	total := metadata.PayloadMetadataBatchSize*2 + 5
	devices := make([]metadata.DiscoveredDeviceMetadata, 0, total)
	for i := 0; i < total; i++ {
		devices = append(devices, metadata.DiscoveredDeviceMetadata{
			AutodiscoveryID: "ad-1", RunID: "run-1", IPAddress: fmt.Sprintf("10.0.0.%d", i),
		})
	}
	require.NoError(t, r.ReportDevices("default", devices))

	got := sender.recorded()
	require.Len(t, got, 3)
	assert.Len(t, got[0].DiscoveredDevices, metadata.PayloadMetadataBatchSize)
	assert.Len(t, got[1].DiscoveredDevices, metadata.PayloadMetadataBatchSize)
	assert.Len(t, got[2].DiscoveredDevices, 5)

	// All batches from a single ReportDevices call must carry the same
	// CollectTimestamp.
	assert.Equal(t, got[0].CollectTimestamp, got[1].CollectTimestamp)
	assert.Equal(t, got[0].CollectTimestamp, got[2].CollectTimestamp)

	// Every device is sent exactly once, in order.
	seen := 0
	for _, p := range got {
		for _, d := range p.DiscoveredDevices {
			assert.Equal(t, devices[seen].IPAddress, d.IPAddress)
			seen++
		}
	}
	assert.Equal(t, total, seen)
}

func TestReportDevicesEmptyIsNoOp(t *testing.T) {
	sender := &fakeSender{}
	r := newTestReporter(t, sender)
	require.NoError(t, r.ReportDevices("default", nil))
	assert.Empty(t, sender.recorded())
}

func TestReportRun(t *testing.T) {
	sender := &fakeSender{}
	r := newTestReporter(t, sender)

	run := metadata.AutodiscoveryRunMetadata{
		AutodiscoveryID:  "ad-1",
		RunID:            "run-1",
		Status:           metadata.AutodiscoveryRunCompleted,
		AddressesScanned: 256,
		StartedAtMs:      1699000000000,
		FinishedAtMs:     1700000000000,
	}
	require.NoError(t, r.ReportRun("prod", run))

	got := sender.recorded()
	require.Len(t, got, 1)
	assert.Equal(t, "prod", got[0].Namespace)
	require.Len(t, got[0].AutodiscoveryRuns, 1)
	assert.Equal(t, run, got[0].AutodiscoveryRuns[0])
	assert.Empty(t, got[0].DiscoveredDevices)
}

func TestReportPropagatesSendError(t *testing.T) {
	sender := &fakeSender{err: errors.New("forwarder is full")}
	r := newTestReporter(t, sender)

	err := r.ReportDevices("default", []metadata.DiscoveredDeviceMetadata{{IPAddress: "10.0.0.1"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "forwarder is full")

	err = r.ReportRun("default", metadata.AutodiscoveryRunMetadata{RunID: "run-1"})
	require.Error(t, err)
}
