// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2025 Datadog, Inc.

//go:build linux_bpf

package dyninst_test

import (
	"context"
	"errors"
	"io"
	"net/http/httptest"
	"net/url"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
)

// TestDecoderErrorHandling tests that decoder errors do not cause the
// entire subsystem to fail and shut down.
func TestDecoderErrorHandling(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)
	tmpDir, cleanup := dyninsttest.PrepTmpDir(t, "decoder-error-handling-test")
	defer cleanup()

	diagCh := make(chan []byte, 10)
	backend := &mockBackend{diagPayloadCh: diagCh}
	backendServer := httptest.NewServer(backend)
	t.Cleanup(backendServer.Close)

	cfgs := testprogs.MustGetCommonConfigs(t)
	idx := slices.IndexFunc(cfgs, func(c testprogs.Config) bool {
		return c.GOARCH == runtime.GOARCH
	})
	require.NotEqual(t, -1, idx)
	cfg := cfgs[idx]

	const binaryName = "rc_tester"

	sampleServicePath := testprogs.MustGetBinary(t, binaryName, cfg)
	_, sampleServicePID, serverPort, err := startSampleService(t, sampleServiceConfig{
		binaryPath: sampleServicePath,
		tmpDir:     tmpDir,
	})
	require.NoError(t, err)
	probes := testprogs.MustGetProbeDefinitions(t, binaryName)

	loader, err := loader.NewLoader()
	require.NoError(t, err)
	actuator := actuator.NewActuator(loader)
	scraper := &mockScraper{}
	logsURL, err := url.Parse(backendServer.URL + "/logs")
	require.NoError(t, err)
	diagURL, err := url.Parse(backendServer.URL + "/diags")
	require.NoError(t, err)

	symdbURL, err := url.Parse("http://dummy-symdb-url")
	require.NoError(t, err)
	c := module.NewController(
		actuator,
		uploader.NewLogsUploaderFactory(uploader.WithURL(logsURL)),
		uploader.NewDiagnosticsUploader(uploader.WithURL(diagURL)),
		symdbURL,
		scraper,
		&failOnceDecoderFactory{
			underlying: module.DefaultDecoderFactory{},
		},
		irgen.NewGenerator(),
	)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.Run(ctx, 10*time.Millisecond)
	}()
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})

	for _, probe := range probes {
		setSnapshotsPerSecond(t, probe, 100)
	}

	scraper.setUpdates([]rcscrape.ProcessUpdate{
		{
			ProcessUpdate: procmon.ProcessUpdate{
				ProcessID: procmon.ProcessID{PID: int32(sampleServicePID)},
				Executable: procmon.Executable{
					Path: sampleServicePath,
				},
				Service: "rc_tester",
			},
			RuntimeID: "foo",
			Probes:    probes,
		},
	})

	// This will result in one decoding failure and then the probes will
	// continue to emit and so we'll eventually go to the expected number
	// of logs.
	const (
		httpHandlerProbeID      = "http_handler"
		lookAtTheRequestProbeID = "look_at_the_request"
	)
	waitForProbeStatus(
		t, diagCh,
		makeTargetStatus(
			uploader.StatusInstalled,
			lookAtTheRequestProbeID,
			httpHandlerProbeID,
		),
	)
	sendTestRequests(t, serverPort, 1)
	// The http_handler comes first because it's called first so it'll fail
	// and then the look_at_the_request probe will emit.
	waitForProbeStatus(t, diagCh, map[string]uploader.Status{
		httpHandlerProbeID:      uploader.StatusError,
		lookAtTheRequestProbeID: uploader.StatusEmitting,
	})
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sendTestRequests(t, serverPort, 1)
			case <-ctx.Done():
				return
			}
		}
	}()
	// They'll both still emit.
	waitForLogMessages(t, backend, 10, "", false /* rewrite */)
}

type mockScraper struct {
	mu struct {
		sync.Mutex
		updates []rcscrape.ProcessUpdate
	}
}

func (s *mockScraper) GetUpdates() []rcscrape.ProcessUpdate {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.mu.updates)
}

func (s *mockScraper) setUpdates(updates []rcscrape.ProcessUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.mu.updates = updates
}

type failOnceDecoderFactory struct {
	underlying module.DecoderFactory
	failed     atomic.Bool
}

func (f *failOnceDecoderFactory) NewDecoder(
	program *ir.Program,
	executable procmon.Executable,
) (module.Decoder, error) {
	decoder, err := f.underlying.NewDecoder(program, executable)
	if err != nil {
		return nil, err
	}
	if !f.failed.CompareAndSwap(false, true) {
		return decoder, nil
	}
	return &failOnceDecoder{
		underlying: decoder,
	}, nil
}

type failOnceDecoder struct {
	underlying module.Decoder
	failed     atomic.Bool
}

func (d *failOnceDecoder) Decode(
	event decode.Event, symbolicator symbol.Symbolicator, out io.Writer,
) (ir.ProbeDefinition, error) {
	probe, err := d.underlying.Decode(event, symbolicator, out)
	if err == nil && d.failed.CompareAndSwap(false, true) {
		err = errors.New("boom")
	}
	return probe, err
}
