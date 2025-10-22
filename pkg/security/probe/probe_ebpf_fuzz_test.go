// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && test

// Package probe holds probe related files
package probe

import (
	"encoding/binary"
	"runtime"
	"testing"
	"time"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	nooptagger "github.com/DataDog/datadog-agent/comp/core/tagger/impl-noop"

	"github.com/DataDog/datadog-agent/pkg/security/config"
	pconfig "github.com/DataDog/datadog-agent/pkg/security/probe/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// createProbe creates a minimal EBPFProbe for fuzzing handleEvent
// Only the fields accessed by handleEvent are initialized to avoid panics
func createProbe(t *testing.T) (*EBPFProbe, error) {
	cfg := &config.Config{}
	cfg.RuntimeSecurity = &config.RuntimeSecurityConfig{}
	cfg.Probe = &pconfig.Config{}
	cfg.Probe.CustomSensitiveWords = []string{"password"}
	// Need non 0 default values for the probe to start
	cfg.RuntimeSecurity.UserSessionsCacheSize = 1000
	cfg.RuntimeSecurity.RuntimeEnabled = true

	tagger := nooptagger.NewComponent()

	opts := Opts{
		StatsdClient:       &statsd.NoOpClient{},
		DontDiscardRuntime: false,
		EBPFLessEnabled:    false, // explicitly fuzz the eBPF probe
		Tagger:             tagger,
	}

	// TODO: what should we set up here? It's required to run otherwise it panics on a nil pointer
	// as it's expected to not be nil in the probe constructor
	ipcComp := ipcmock.New(t)
	pr, err := NewProbe(cfg, ipcComp, opts)
	if err != nil {
		return nil, err
	}

	return pr.PlatformProbe.(*EBPFProbe), nil
}

// makeSeedEventData creates a minimal valid event data buffer for seeding
// Event format: timestamp (8 bytes) + type (4 bytes) + flags (4 bytes)
func makeSeedEventData(eventType model.EventType) []byte {
	buf := make([]byte, 16)
	// This is going to create a seed value that is different on each run, that could be helpful if there's
	// any freshness checks in the code.
	binary.NativeEndian.PutUint64(buf[0:8], uint64(time.Now().UnixNano()))
	binary.NativeEndian.PutUint32(buf[8:12], uint32(eventType))
	binary.NativeEndian.PutUint32(buf[12:16], 0) // flags
	return buf
}

// To run the fuzzer `go test -fuzz=FuzzHandleEvent -run=FuzzHandleEvent`
func FuzzHandleEvent(f *testing.F) {
	// Seed corpus with minimal valid events for common event types
	// These seeds provide starting points for the fuzzer to mutate
	f.Add(makeSeedEventData(model.FileOpenEventType))
	f.Add(makeSeedEventData(model.ExecEventType))
	f.Add(makeSeedEventData(model.FileMkdirEventType))
	f.Add(makeSeedEventData(model.FileUnlinkEventType))
	f.Add(makeSeedEventData(model.FileRenameEventType))
	f.Add(makeSeedEventData(model.FileChmodEventType))
	f.Add(makeSeedEventData(model.FileChownEventType))
	f.Add(makeSeedEventData(model.ExitEventType))
	f.Add(makeSeedEventData(model.FileMountEventType))
	f.Add(makeSeedEventData(model.FileUmountEventType))

	// Edge cases: empty data, minimal data, invalid event types
	f.Add([]byte{})
	f.Add([]byte{1, 2, 3})
	f.Add(make([]byte, 16))
	f.Add(makeSeedEventData(model.MaxKernelEventType + 1))

	numCPU := runtime.NumCPU()
	f.Fuzz(func(t *testing.T, data []byte) {
		p, err := createProbe(t)
		if err != nil {
			t.Fatalf("failed to create minimal probe: %v", err)
		}
		defer p.cancelFnc()
		p.handleEvent(numCPU, data)
	})
}
