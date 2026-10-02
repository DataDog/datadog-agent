// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package transaction

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"
)

func TestBytesPayloadCaptureIsLocal(t *testing.T) {
	payload := NewBytesPayload([]byte("original wire body"), 3)
	payload.Destination = PrimaryOnly
	before, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	metadata := &CaptureMetadata{SessionID: "private-capture-session", CycleID: 11, PayloadID: 7, Ordinals: []uint64{1, 5}}
	payload.SetCapture(metadata)
	if payload.Capture() != metadata {
		t.Fatal("capture metadata was not attached")
	}
	after, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !bytes.Equal(payload.GetContent(), []byte("original wire body")) || payload.GetPointCount() != 3 || payload.Destination != PrimaryOnly {
		t.Fatal("capture changed serialized payload or delivery metadata")
	}
	payload.ClearCapture()
	if payload.Capture() != nil {
		t.Fatal("cleared payload retained capture state")
	}
}

func TestBytesPayloadCaptureConcurrentDetach(t *testing.T) {
	payload := NewBytesPayloadWithoutMetaData([]byte("body"))
	metadata := &CaptureMetadata{SessionID: "private-capture-session", CycleID: 11, PayloadID: 7}
	var workers sync.WaitGroup
	for range 4 {
		workers.Go(func() {
			for range 1000 {
				payload.SetCapture(metadata)
				if got := payload.Capture(); got != nil && got != metadata {
					t.Error("read an unexpected capture pointer")
				}
				payload.ClearCapture()
			}
		})
	}
	workers.Wait()
	payload.ClearCapture()
	if payload.Capture() != nil {
		t.Fatal("capture remained attached")
	}
}
