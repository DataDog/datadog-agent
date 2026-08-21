// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build ignore

// fix_fixture rewrites metrics_capture.zstd so that TaggerState.State map keys
// use full entity-ID strings (e.g. "container_id://abc123") instead of bare IDs
// (e.g. "abc123").  Run once from the repo root:
//
//	go run ./test/new-e2e/tests/agent-subcommands/dogstatsdreplay/fixtures/fix_fixture.go
package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"

	"github.com/DataDog/zstd"
	"google.golang.org/protobuf/proto"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
)

const fixturePath = "test/new-e2e/tests/agent-subcommands/dogstatsdreplay/fixtures/metrics_capture.zstd"

func main() {
	compressed, err := os.ReadFile(fixturePath)
	if err != nil {
		fatalf("read fixture: %v", err)
	}

	data, err := zstd.Decompress(nil, compressed)
	if err != nil {
		fatalf("decompress: %v", err)
	}

	L := len(data)
	if L < 12 {
		fatalf("file too short (%d bytes)", L)
	}

	// File layout (decompressed):
	//   [8-byte header] [messages...] [4-byte zero separator] [state proto] [4-byte state size LE]
	stateSz := int(binary.LittleEndian.Uint32(data[L-4 : L]))
	if stateSz == 0 {
		fmt.Println("no tagger state in fixture, nothing to fix")
		return
	}

	stateStart := L - stateSz - 4 // start of proto bytes
	separatorStart := stateStart - 4

	state := &pb.TaggerState{}
	if err := proto.Unmarshal(data[stateStart:L-4], state); err != nil {
		fatalf("unmarshal TaggerState: %v", err)
	}

	fmt.Printf("PidMap entries: %d\n", len(state.PidMap))
	fmt.Printf("State entries:  %d\n", len(state.State))

	// Build a mapping from bare-ID → full entity string using PidMap values.
	// PidMap values are already full strings like "container_id://abc123".
	bareToFull := make(map[string]string)
	for _, fullID := range state.PidMap {
		idx := strings.Index(fullID, "://")
		if idx < 0 {
			continue
		}
		bareID := fullID[idx+3:]
		bareToFull[bareID] = fullID
	}

	// Rebuild State map with full entity ID keys.
	fixed := make(map[string]*pb.Entity, len(state.State))
	fixCount := 0
	for k, v := range state.State {
		if full, ok := bareToFull[k]; ok {
			fixed[full] = v
			fixCount++
			fmt.Printf("  fixed key: %q → %q\n", k, full)
		} else {
			// Key already looks like a full entity string or has no match; keep as-is.
			fixed[k] = v
		}
	}
	fmt.Printf("Fixed %d/%d State entries\n", fixCount, len(state.State))

	state.State = fixed

	newStateBytes, err := proto.Marshal(state)
	if err != nil {
		fatalf("marshal fixed TaggerState: %v", err)
	}

	// Reassemble: prefix + separator + new state + new size
	prefix := data[:separatorStart]
	separator := []byte{0, 0, 0, 0}
	sizeBuf := make([]byte, 4)
	binary.LittleEndian.PutUint32(sizeBuf, uint32(len(newStateBytes)))

	var reassembled []byte
	reassembled = append(reassembled, prefix...)
	reassembled = append(reassembled, separator...)
	reassembled = append(reassembled, newStateBytes...)
	reassembled = append(reassembled, sizeBuf...)

	recompressed, err := zstd.Compress(nil, reassembled)
	if err != nil {
		fatalf("recompress: %v", err)
	}

	if err := os.WriteFile(fixturePath, recompressed, 0644); err != nil {
		fatalf("write fixture: %v", err)
	}

	fmt.Printf("Wrote %d bytes (compressed) to %s\n", len(recompressed), fixturePath)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}
