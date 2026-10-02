// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package telemetry

import (
	"bytes"
	"testing"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/gogo/protobuf/proto"

	"github.com/DataDog/datadog-agent/cmd/eudm-simulator/internal/schema"
)

func TestProcessEncodingPreservesNativeHintOneof(t *testing.T) {
	for _, hint := range []int32{0, 1, 3} {
		input := &model.CollectorProc{HostName: "native-laptop", GroupId: 11, GroupSize: 1,
			Hints:     &model.CollectorProc_HintMask{HintMask: hint},
			Info:      &model.SystemInfo{TotalMemory: 16 << 30, Os: &model.OSInfo{Name: "darwin"}},
			Processes: []*model.Process{{Pid: 42, CreateTime: -9007199254740993, Command: &model.Command{Comm: "Code", Args: []string{"code", "--token=********"}}}}}
		data, err := Encode(input)
		if err != nil {
			t.Fatal(err)
		}
		out, err := Decode(schema.Processes, data)
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(input, out.Processes) {
			t.Fatal("protobuf JSON lost hints, timing precision, or process fields")
		}
		if _, err := Decode(schema.Processes, append(data, []byte(` {}`)...)); err == nil {
			t.Fatal("concatenated process documents accepted")
		}
		invalid := bytes.Replace(data, []byte(`"hintMask":`), []byte(`"unsupported_hint":`), 1)
		if _, err := Decode(schema.Processes, invalid); err == nil {
			t.Fatal("unknown protobuf field accepted")
		}
	}
}

func TestConnectionEncodingPreservesEncodedTablesAndCounters(t *testing.T) {
	input := &model.CollectorConnections{HostName: "native-laptop", GroupSize: 1, EncodedTags: []byte{1, 2, 3},
		Connections: []*model.Connection{{Pid: 42, Laddr: &model.Addr{Ip: "192.0.2.1"}, Raddr: &model.Addr{Ip: "198.51.100.2", Port: 443}, LastBytesSent: 9007199254740993}}}
	data, err := Encode(input)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Decode(schema.Connections, data)
	if err != nil || !proto.Equal(input, out.Connections) {
		t.Fatalf("connection protobuf fields changed: %v", err)
	}
	if _, err := Decode(schema.Connections, []byte(`null`)); err == nil {
		t.Fatal("null connection sample accepted")
	}
}
