// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package resources

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/genresources"
)

const testUUID = "GPU-12345678-1234-5678-1234-111111111111"

type recordingSender struct{ payloads [][]byte }

func (r *recordingSender) EventPlatformEvent(payload []byte, track string) {
	if track != "genresources" {
		panic("incorrect event track")
	}
	r.payloads = append(r.payloads, payload)
}

func decodeEvent(t *testing.T, payload []byte) (*genresources.GenericResourceEvent, map[string]any) {
	t.Helper()
	var event genresources.GenericResourceEvent
	require.NoError(t, proto.Unmarshal(payload, &event))
	var fields map[string]any
	require.NoError(t, json.Unmarshal(event.Resource.FieldsJson, &fields))
	return &event, fields
}

func newTestPublisher(t *testing.T) *Publisher {
	t.Helper()
	p, err := NewPublisher(Options{TTL: 15 * time.Minute, MaxPerRun: 64})
	require.NoError(t, err)
	return p
}

func TestPublisherEnvelopeAndRefresh(t *testing.T) {
	p := newTestPublisher(t)
	sender := &recordingSender{}
	now := time.Unix(1700000000, 123).UTC()
	clique := uint32(0)
	device := Device{UUID: testUUID, Model: "tesla_t4", FabricCliqueID: &clique}
	require.NoError(t, p.Publish(now, "canonical-host", []Device{device}, sender))
	require.Len(t, sender.payloads, 1)
	event, fields := decodeEvent(t, sender.payloads[0])
	require.Equal(t, "integrations-core", event.Source)
	require.Equal(t, "gpu_device", event.Resource.Type)
	require.Equal(t, strings.ToLower(testUUID), event.Resource.Key)
	require.Equal(t, event.Resource.Key, fields["gpu_uuid"])
	require.Equal(t, "canonical-host", fields["hostname"])
	require.Equal(t, float64(0), fields["gpu_fabric_clique_id"])
	require.NotContains(t, fields, "gpu_peers")
	require.NotContains(t, fields, "gpu_numa_node")
	require.NotContains(t, fields, "memory_total_bytes")
	require.Equal(t, now, event.Resource.SeenAt.AsTime())
	require.Equal(t, now.Add(15*time.Minute), event.Resource.ExpireAt.AsTime())
	require.NoError(t, p.Publish(now.Add(3*time.Minute), "canonical-host", []Device{device}, sender))
	require.Len(t, sender.payloads, 1)
	require.NoError(t, p.Publish(now.Add(4*time.Minute), "canonical-host", []Device{device}, sender))
	require.Len(t, sender.payloads, 2)
	refreshed, _ := decodeEvent(t, sender.payloads[1])
	require.Equal(t, now.Add(4*time.Minute), refreshed.Resource.SeenAt.AsTime())
}

func TestPublisherChangesAndDroppedFields(t *testing.T) {
	p := newTestPublisher(t)
	sender := &recordingSender{}
	now := time.Unix(1700000000, 0)
	d := Device{UUID: testUUID, Model: "tesla_t4", DriverVersion: "580.1"}
	require.NoError(t, p.Publish(now, "host", []Device{d}, sender))
	d.DriverVersion = ""
	require.NoError(t, p.Publish(now.Add(time.Second), "host", []Device{d}, sender))
	require.Len(t, sender.payloads, 2, "a dropped field is a change")
	_, fields := decodeEvent(t, sender.payloads[1])
	require.NotContains(t, fields, "gpu_driver_version")
	require.Equal(t, "tesla_t4", fields["gpu_device"])
	d.Model = "new_model"
	require.NoError(t, p.Publish(now.Add(2*time.Second), "host", []Device{d}, sender))
	_, fields = decodeEvent(t, sender.payloads[2])
	require.Equal(t, "new_model", fields["gpu_device"])
	require.NoError(t, p.Publish(now.Add(3*time.Second), "new-host", []Device{d}, sender))
	_, fields = decodeEvent(t, sender.payloads[3])
	require.Equal(t, "new-host", fields["hostname"])
	require.NoError(t, p.Publish(now.Add(4*time.Second), "new-host", nil, sender))
	require.Empty(t, p.last)
	require.Len(t, sender.payloads, 4, "disappearance must not emit a tombstone")
}

func TestPublisherClearsFabricFieldsWhenDeviceLeavesDomain(t *testing.T) {
	p := newTestPublisher(t)
	sender := &recordingSender{}
	now := time.Unix(1700000000, 0)
	clique := uint32(3)
	d := Device{
		UUID: testUUID, Model: "h100",
		FabricClusterUUID: "cluster-1", FabricCliqueID: &clique, NVLinkDomain: "cluster-1:3",
	}
	require.NoError(t, p.Publish(now, "host", []Device{d}, sender))
	_, fields := decodeEvent(t, sender.payloads[0])
	require.Equal(t, "cluster-1:3", fields["nvlink_domain"])

	d.FabricClusterUUID, d.FabricCliqueID, d.NVLinkDomain = "", nil, ""
	require.NoError(t, p.Publish(now.Add(time.Second), "host", []Device{d}, sender))
	require.Len(t, sender.payloads, 2)
	_, fields = decodeEvent(t, sender.payloads[1])
	require.NotContains(t, fields, "gpu_fabric_cluster_uuid")
	require.NotContains(t, fields, "gpu_fabric_clique_id")
	require.NotContains(t, fields, "nvlink_domain")
	require.Equal(t, "h100", fields["gpu_device"])
}

func TestPublisherReportsPayloadFailures(t *testing.T) {
	now := time.Unix(1700000000, 0)
	sender := &recordingSender{}
	p := newTestPublisher(t)
	err := p.Publish(now, "host", []Device{{UUID: testUUID, Model: strings.Repeat("x", maxFieldsBytes)}}, sender)
	require.ErrorContains(t, err, strings.ToLower(testUUID))
	require.ErrorContains(t, err, "exceeding limit")
	require.Empty(t, sender.payloads)
	require.NoError(t, p.Publish(now, "host", []Device{{UUID: testUUID}}, sender))
	require.Len(t, sender.payloads, 1, "failure must not mark the resource submitted")
}

func TestPublisherJoinsFailuresAndContinues(t *testing.T) {
	sender := &recordingSender{}
	p := newTestPublisher(t)
	oversized := strings.Repeat("x", maxFieldsBytes)
	err := p.Publish(time.Unix(1700000000, 0), "host", []Device{
		{UUID: "gpu-a", Model: oversized},
		{UUID: "gpu-b", Model: oversized},
		{UUID: "gpu-c"},
	}, sender)
	require.ErrorContains(t, err, "GPU gpu-a:")
	require.ErrorContains(t, err, "GPU gpu-b:")
	require.Len(t, sender.payloads, 1)
	event, _ := decodeEvent(t, sender.payloads[0])
	require.Equal(t, "gpu-c", event.Resource.Key)
}

func TestPublisherTimingOptions(t *testing.T) {
	for _, options := range []Options{
		{TTL: -time.Minute, MaxPerRun: 64},
		{TTL: time.Minute, RefreshInterval: time.Minute, MaxPerRun: 64},
		{TTL: time.Minute, MaxPerRun: -1},
	} {
		_, err := NewPublisher(options)
		require.Error(t, err)
	}
	p, err := NewPublisher(Options{TTL: 8 * time.Minute, RefreshInterval: time.Minute, MaxPerRun: 2})
	require.NoError(t, err)
	sender := &recordingSender{}
	now := time.Unix(1700000000, 0)
	devices := []Device{{UUID: "gpu-1"}, {UUID: "gpu-2"}, {UUID: "gpu-3"}}
	require.NoError(t, p.Publish(now, "host", devices, sender))
	require.Len(t, sender.payloads, 2)
	event, _ := decodeEvent(t, sender.payloads[0])
	require.Equal(t, 8*time.Minute, event.Resource.ExpireAt.AsTime().Sub(event.Resource.SeenAt.AsTime()))
	require.NoError(t, p.Publish(now.Add(time.Minute), "host", devices[:2], sender))
	require.Len(t, sender.payloads, 4)
}

func TestPublisherBoundsSubmissionsWithoutStarvingDevices(t *testing.T) {
	p := newTestPublisher(t)
	sender := &recordingSender{}
	devices := make([]Device, 64+7)
	for i := range devices {
		devices[i].UUID = fmt.Sprintf("GPU-12345678-1234-5678-1234-%012d", i+1)
	}
	now := time.Unix(1700000000, 0)
	require.NoError(t, p.Publish(now, "host", devices, sender))
	require.Len(t, sender.payloads, 64)
	require.NoError(t, p.Publish(now.Add(time.Second), "host", devices, sender))
	require.Len(t, sender.payloads, len(devices))
	seen := make(map[string]bool)
	for _, payload := range sender.payloads {
		event, _ := decodeEvent(t, payload)
		require.False(t, seen[event.Resource.Key])
		seen[event.Resource.Key] = true
	}
}

func TestPublicPythonWireFixture(t *testing.T) {
	// Generated with integrations-core's genericresource_pb2.py, not the Go
	// binding under test; protects the externally shared protobuf field numbers.
	payload, err := os.ReadFile("testdata/python_event.pb")
	require.NoError(t, err)
	event, fields := decodeEvent(t, payload)
	require.Equal(t, "agent", event.Source)
	require.Equal(t, "gpu_device", event.Resource.Type)
	require.Equal(t, strings.ToLower(testUUID), event.Resource.Key)
	require.Equal(t, "canonical-host", fields["hostname"])
	require.Equal(t, int64(1700000000), event.Resource.SeenAt.Seconds)
	require.Equal(t, int64(1700003600), event.Resource.ExpireAt.Seconds)
	roundTrip, err := proto.Marshal(event)
	require.NoError(t, err)
	require.Equal(t, payload, roundTrip)
}
