// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcscrape_test

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dispatcher"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcjson"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestMain(m *testing.M) {
	dyninsttest.SetupLogging()
	goleak.VerifyTestMain(m, goleak.IgnoreCurrent())
}

// TestScrapeRemoteConfig tests that the scraper can scrape remote config
// files from a process. It runs the test for both the v1 and v2 versions of
// the dd-trace-go library. It runs them in parallel because they each have
// a 5s timeout.
func TestScrapeRemoteConfig(t *testing.T) {
	if testing.Short() {
		t.Skip("waiting for the 5s loop is slow")
	}

	dyninsttest.SkipIfKernelNotSupported(t)

	cfgs := testprogs.MustGetCommonConfigs(t)

	testCases := []struct {
		program      string
		symdbSupport bool
	}{
		{"rc_tester", true},
		// rc_tester_v1 (using the v1 version of dd-trace-go) does not support
		// reading the remote config key controlling the SymDB upload.
		{"rc_tester_v1", false},
	}
	for _, tc := range testCases {
		for _, cfg := range cfgs {
			t.Run(tc.program+"-"+cfg.String(), func(t *testing.T) {
				if cfg.GOARCH != runtime.GOARCH {
					t.Skipf(
						"cross-execution is not supported, running on %s",
						runtime.GOARCH,
					)
				}
				t.Parallel()
				runScrapeRemoteConfigTest(t, tc.program, cfg, tc.symdbSupport)
			})
		}
	}
}

func runScrapeRemoteConfigTest(
	t *testing.T,
	program string,
	cfg testprogs.Config,
	symDBSupport bool,
) {
	tmpDir, cleanupTmpDir := dyninsttest.PrepTmpDir(
		t, strings.ReplaceAll(t.Name(), "/", "_"),
	)
	defer cleanupTmpDir()

	prog := testprogs.MustGetBinary(t, program, cfg)
	probes := testprogs.MustGetProbeDefinitions(t, program)
	rcHandler := dyninsttest.NewMockAgentRCServer()
	rcServer := httptest.NewServer(rcHandler)
	defer rcServer.Close()
	serverURL, err := url.Parse(rcServer.URL)
	require.NoError(t, err)
	host, port, err := net.SplitHostPort(serverURL.Host)
	require.NoError(t, err)
	env := []string{
		fmt.Sprintf("DD_AGENT_HOST=%s", host),
		fmt.Sprintf("DD_AGENT_PORT=%s", port), // for remote config
		"DD_DYNAMIC_INSTRUMENTATION_ENABLED=true",
		"DD_REMOTE_CONFIGURATION_ENABLED=true",
		"DD_REMOTE_CONFIG_POLL_INTERVAL_SECONDS=.01",
		"DD_SERVICE=rc_tester",
		"DD_ENV=test",
		"DD_VERSION=1.0.0",
		"DD_REMOTE_CONFIG_TUF_NO_VERIFICATION=true",
	}
	childStdout, err := os.Create(path.Join(tmpDir, "child.stdout"))
	require.NoError(t, err)
	childStderr, err := os.Create(path.Join(tmpDir, "child.stderr"))
	require.NoError(t, err)
	child := exec.Command(prog)
	child.Stdout = childStdout
	child.Stderr = childStderr
	child.Env = env
	err = child.Start()
	require.NoError(t, err)
	defer func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	}()
	a := actuator.NewActuator()
	t.Cleanup(func() { require.NoError(t, a.Shutdown()) })
	loader, err := loader.NewLoader()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, loader.Close()) })
	dispatcher := dispatcher.NewDispatcher(loader.OutputReader())
	t.Cleanup(func() { require.NoError(t, dispatcher.Shutdown()) })

	rcScraper := rcscrape.NewScraper(a, dispatcher, loader)

	procMon := procmon.NewProcessMonitor(rcScraper.AsProcMonHandler())
	t.Cleanup(func() { procMon.Close() })
	procMon.NotifyExec(uint32(child.Process.Pid))
	rcsFiles := make(map[string][]byte)
	for _, probe := range probes {
		marshaled, err := json.Marshal(probe)
		require.NoError(t, err)
		rcsFiles[mkPath(t, probe.GetID())] = marshaled
	}
	var symdbPath string
	if symDBSupport {
		symdbPayload := []byte(`{"uploadSymbols": true}`)
		symdbPath = mkPathWithVal("LIVE_DEBUGGING_SYMBOL_DB", "symDb", symdbPayload)
		rcsFiles[symdbPath] = symdbPayload
	}
	rcHandler.UpdateRemoteConfig(rcsFiles)
	waitForExpected(t, rcScraper, append(probes[:0:0], probes...), symDBSupport)

	// Make sure that the scraper handles more updates correctly.
	newUpdate := append(probes[:0:0], probes...)
	for _, probe := range probes {
		var toMarshal ir.ProbeDefinition
		switch p := probe.(type) {
		case *rcjson.SnapshotProbe:
			copied := *p
			copied.ID += "_updated"
			toMarshal = &copied
		default:
			t.Fatalf("unexpected probe type %T", p)
		}

		newUpdate = append(newUpdate, toMarshal)
		marshaled, err := json.Marshal(toMarshal)
		require.NoError(t, err)
		rcsFiles[mkPath(t, probe.GetID())] = marshaled
	}
	rcsFiles[mkPath(t, "empty")] = []byte{}
	// Remove the SymDB key; we'll check that the corresponding flag on the
	// update turns false.
	if symDBSupport {
		delete(rcsFiles, symdbPath)
	}
	rcHandler.UpdateRemoteConfig(rcsFiles)
	waitForExpected(t, rcScraper, newUpdate, false /* expShouldUploadSymDB */)

	// Modify only the SymDB key and check that we get an update.
	if symDBSupport {
		symdbPayload := []byte(`{"uploadSymbols": true}`)
		symdbPath = mkPathWithVal("LIVE_DEBUGGING_SYMBOL_DB", "symDb", symdbPayload)
		rcsFiles[symdbPath] = symdbPayload
		rcHandler.UpdateRemoteConfig(rcsFiles)
		waitForExpected(t, rcScraper, newUpdate, true /* expShouldUploadSymDB */)
	}
}

func waitForExpected(
	t *testing.T, rcScraper *rcscrape.Scraper, exp []ir.ProbeDefinition, expShouldUploadSymDB bool,
) {
	slices.SortFunc(exp, ir.CompareProbeIDs)
	require.Eventually(t, func() bool {
		updates := rcScraper.GetUpdates()
		if len(updates) == 0 {
			return false
		}
		got := updates[0].Probes
		slices.SortFunc(got, ir.CompareProbeIDs)
		assert.Equal(t, exp, got)
		assert.Equal(t, expShouldUploadSymDB, updates[0].ShouldUploadSymDB, "SymDB upload flag doesn't match")
		return true
	}, 10*time.Second, 100*time.Microsecond)
}

func formatConfigPath(
	file data.ConfigPath,
) string {
	// Inverse of data.ParseConfigPath.
	switch file.Source {
	case data.SourceDatadog:
		return fmt.Sprintf(
			"datadog/%d/%s/%s/%s",
			file.OrgID,
			file.Product,
			file.ConfigID,
			file.Name,
		)
	case data.SourceEmployee:
		return fmt.Sprintf(
			"employee/%s/%s/%s",
			file.Product,
			file.ConfigID,
			file.Name,
		)
	default:
		panic(fmt.Errorf("unknown source %v", file.Source))
	}
}

func mkPath(t *testing.T, name string) string {
	configID, err := uuid.NewRandom()
	require.NoError(t, err)
	return formatConfigPath(data.ConfigPath{
		Source:   data.SourceDatadog,
		OrgID:    1234,
		Product:  data.ProductLiveDebugging,
		ConfigID: configID.String(),
		Name:     name,
	})
}

func mkPathWithVal(product data.Product, id string, val []byte) string {
	return formatConfigPath(data.ConfigPath{
		Source:   data.SourceDatadog,
		OrgID:    1234,
		Product:  string(product),
		ConfigID: id,
		Name:     hex.EncodeToString(val),
	})
}

// TestNoDdTraceGo tests that the scraper correctly handles the case where the
// dd-trace-go library is not present in the process. In particular, the Scraper
// should try to attach probes to the process, fail, and then stop tracking the
// process.
func TestNoDdTraceGo(t *testing.T) {
	dyninsttest.SkipIfKernelNotSupported(t)

	cfgs := testprogs.MustGetCommonConfigs(t)
	for _, cfg := range cfgs {
		t.Run(cfg.String(), func(t *testing.T) {
			if cfg.GOARCH != runtime.GOARCH {
				t.Skipf(
					"cross-execution is not supported, running on %s",
					runtime.GOARCH,
				)
			}
			t.Parallel()
			testNoDdTraceGo(t, cfg)
		})
	}
}

func testNoDdTraceGo(t *testing.T, cfg testprogs.Config) {
	prog := testprogs.MustGetBinary(t, "simple", cfg)

	tmpDir, cleanupTmpDir := dyninsttest.PrepTmpDir(t, strings.ReplaceAll(t.Name(), "/", "_"))
	defer cleanupTmpDir()

	a := actuator.NewActuator()
	t.Cleanup(func() { require.NoError(t, a.Shutdown()) })
	type irGenFailedMessage struct {
		executablePath string
		err            error
	}
	irGenFailureCh := make(chan irGenFailedMessage)
	irGenerator := irGenFunc(func(
		programID ir.ProgramID, executablePath string, probes []ir.ProbeDefinition,
	) (*ir.Program, error) {
		p, err := (rcscrape.IRGeneratorImpl{}).GenerateIR(programID, executablePath, probes)
		if err != nil {
			irGenFailureCh <- irGenFailedMessage{executablePath: executablePath, err: err}
		}
		return p, err
	})
	l, err := loader.NewLoader()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, l.Close()) })
	d := dispatcher.NewDispatcher(l.OutputReader())
	t.Cleanup(func() { require.NoError(t, d.Shutdown()) })

	rcScraper := rcscrape.NewScraperWithIRGenerator(a, d, l, irGenerator)

	stdout, err := os.Create(path.Join(tmpDir, "child.stdout"))
	require.NoError(t, err)
	stderr, err := os.Create(path.Join(tmpDir, "child.stderr"))
	require.NoError(t, err)
	child := exec.Command(prog)
	child.Stdout = stdout
	child.Stderr = stderr
	child.Env = []string{
		"DD_SERVICE=simple",
		"DD_DYNAMIC_INSTRUMENTATION_ENABLED=true",
		"DD_REMOTE_CONFIGURATION_ENABLED=true",
	}
	stdin, err := child.StdinPipe()
	require.NoError(t, err)
	err = child.Start()
	require.NoError(t, err)
	defer func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	}()

	rcScraper.AsProcMonHandler().HandleUpdate(procmon.ProcessesUpdate{
		Processes: []procmon.ProcessUpdate{
			{
				ProcessID:  procmon.ProcessID{PID: int32(child.Process.Pid)},
				Executable: procmon.Executable{Path: prog},
				Service:    "simple",
			},
		},
	})
	require.Eventually(t, func() bool {
		processes := rcScraper.GetTrackedProcesses()
		return len(processes) > 0
	}, 1*time.Second, 10*time.Millisecond)
	msg := <-irGenFailureCh
	require.Equal(t, prog, msg.executablePath)
	require.Eventually(t, func() bool {
		processes := rcScraper.GetTrackedProcesses()
		return len(processes) == 0
	}, 1*time.Second, 10*time.Millisecond)

	require.NoError(t, stdin.Close())
	require.NoError(t, child.Wait())
}

type irGenFunc func(
	programID ir.ProgramID,
	executablePath string,
	probes []ir.ProbeDefinition,
) (*ir.Program, error)

func (f irGenFunc) GenerateIR(
	programID ir.ProgramID, executablePath string, probes []ir.ProbeDefinition,
) (*ir.Program, error) {
	return f(programID, executablePath, probes)
}
