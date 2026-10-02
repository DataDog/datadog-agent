// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

func TestEmptyGroupChunksKeepSingletonValidationStrict(t *testing.T) {
	for _, test := range []struct {
		stream schema.Stream
		value  any
	}{
		{schema.Processes, &model.CollectorProc{HostName: "capture-host", GroupSize: 2, Info: &model.SystemInfo{TotalMemory: 8 << 30, Os: &model.OSInfo{Name: "darwin"}}}},
		{schema.Connections, &model.CollectorConnections{HostName: "capture-host", GroupSize: 2}},
	} {
		t.Run(string(test.stream), func(t *testing.T) {
			data, err := Encode(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Decode(test.stream, data); err == nil {
				t.Fatal("empty standalone sample was accepted")
			}
			if _, err := DecodeGroupChunk(test.stream, data); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGroupChunkStillValidatesRequiredShape(t *testing.T) {
	for _, test := range []struct {
		stream schema.Stream
		data   string
	}{
		{schema.Metrics, `{}`},
		{schema.Processes, `{"hostName":"capture-host","groupSize":2}`},
		{schema.Connections, `{"groupSize":2}`},
		{schema.Connections, `{"hostName":"capture-host","connections":[null]}`},
		{schema.Connections, `{"hostName":"capture-host","connections":[{"pid":100}]}`},
		{schema.Connections, `{"hostName":"capture-host","unknown":"value"}`},
	} {
		if _, err := DecodeGroupChunk(test.stream, []byte(test.data)); err == nil {
			t.Fatal("invalid grouped sample was accepted")
		}
	}
}

func TestHostMetadataPreservesNativeHardwareJSONShape(t *testing.T) {
	const hardware = `{"network":{"interfaces":[{"name":"en0","ipv4":["192.0.2.4"]}]},"filesystem":[{"name":"/dev/disk3s1","size":9007199254740993}]}`
	value := &HostMetadata{Hostname: "native-host", AgentVersion: "7.85.0", OS: "darwin", Gohai: hardware}
	data, err := Encode(value)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(schema.HostMetadata, data)
	if err != nil || decoded.HostMetadata.Gohai != hardware {
		t.Fatalf("native hardware JSON shape or precision changed: %v", err)
	}
}
