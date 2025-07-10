// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package rcscrape_test

import (
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

	"github.com/DataDog/datadog-agent/pkg/config/remote/data"
	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/dyninsttest"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/irgen"
	"github.com/DataDog/datadog-agent/pkg/dyninst/loader"
	"github.com/DataDog/datadog-agent/pkg/dyninst/procmon"
	"github.com/DataDog/datadog-agent/pkg/dyninst/rcscrape"
	"github.com/DataDog/datadog-agent/pkg/dyninst/testprogs"
)

func TestMain(m *testing.M) {
	dyninsttest.SetupLogging()
	os.Exit(m.Run())
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
	for _, program := range []string{"rc_tester", "rc_tester_v1"} {
		for _, cfg := range cfgs {
			t.Run(program+"-"+cfg.String(), func(t *testing.T) {
				if cfg.GOARCH != runtime.GOARCH {
					t.Skipf(
						"cross-execution is not supported, running on %s",
						runtime.GOARCH,
					)
				}
				t.Parallel()
				runScrapeRemoteConfigTest(t, program, cfg)
			})
		}
	}
}

func runScrapeRemoteConfigTest(t *testing.T, program string, cfg testprogs.Config) {
	var cleanupFuncs []func()
	cleanup := func() {
		for _, f := range cleanupFuncs {
			f()
		}
	}
	defer func() {
		if t.Failed() {
			cleanup()
		}
	}()
	tmpDir, cleanupTmpDir := dyninsttest.PrepTmpDir(
		t, strings.ReplaceAll(t.Name(), "/", "_"),
	)
	cleanupFuncs = append(cleanupFuncs, cleanupTmpDir)

	prog := testprogs.MustGetBinary(t, program, cfg)
	probes := testprogs.MustGetProbeDefinitions(t, program)
	rcHandler := dyninsttest.NewMockAgentRCServer()
	rcServer := httptest.NewServer(rcHandler)
	cleanupFuncs = append(cleanupFuncs, rcServer.Close)
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
	cleanupFuncs = append(cleanupFuncs, func() {
		_ = child.Process.Kill()
		_ = child.Wait()
	})
	loader, err := loader.NewLoader()
	require.NoError(t, err)
	a := actuator.NewActuator(loader)
	rcScraper := rcscrape.NewScraper(a)

	procMon := procmon.NewProcessMonitor(rcScraper.AsProcMonHandler())
	procMon.NotifyExec(uint32(child.Process.Pid))
	rcsFiles := make(map[string][]byte)
	for _, probe := range probes {
		marshaled, err := json.Marshal(probe)
		require.NoError(t, err)
		rcsFiles[mkPath(t, probe.GetID())] = marshaled
	}
	rcHandler.UpdateRemoteConfig(rcsFiles)
	exp := append(probes[:0:0], probes...)
	slices.SortFunc(exp, ir.CompareProbeIDs)
	require.Eventually(t, func() bool {
		updates := rcScraper.GetUpdates()
		if len(updates) == 0 {
			return false
		}
		got := updates[0].Probes
		slices.SortFunc(got, ir.CompareProbeIDs)
		return assert.Equal(t, exp, got)
	}, 10*time.Second, 100*time.Millisecond)
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

	loader, err := loader.NewLoader()
	require.NoError(t, err)
	a := actuator.NewActuator(loader)
	irGenFailureCh := make(chan irGenFailedMessage)
	rcScraper := rcscrape.NewScraper(&wrappedActuator{
		inner:          a,
		irGenFailureCh: irGenFailureCh,
	})

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

	procMon := procmon.NewProcessMonitor(rcScraper.AsProcMonHandler())
	procMon.NotifyExec(uint32(child.Process.Pid))
	require.Eventually(t, func() bool {
		processes := rcScraper.GetTrackedProcesses()
		return len(processes) > 0
	}, 1*time.Second, 10*time.Millisecond)
	msg := <-irGenFailureCh
	require.Equal(t, int(msg.processID.PID), child.Process.Pid)
	require.Eventually(t, func() bool {
		processes := rcScraper.GetTrackedProcesses()
		return len(processes) == 0
	}, 1*time.Second, 10*time.Millisecond)

	require.NoError(t, stdin.Close())
	require.NoError(t, child.Wait())
}

type wrappedActuator struct {
	inner          *actuator.Actuator
	irGenFailureCh chan irGenFailedMessage
}

type irGenFailedMessage struct {
	processID actuator.ProcessID
	err       error
	probes    []ir.ProbeDefinition
}

type noDdTraceGoReporter struct {
	actuator.Reporter
	irGenFailureCh chan irGenFailedMessage
}

func (wa *wrappedActuator) NewTenant(
	name string,
	reporter actuator.Reporter,
	opts ...irgen.Option,
) *actuator.Tenant {
	return wa.inner.NewTenant(name, &noDdTraceGoReporter{
		Reporter:       reporter,
		irGenFailureCh: wa.irGenFailureCh,
	}, opts...)
}

func (r *noDdTraceGoReporter) ReportIRGenFailed(
	processID actuator.ProcessID,
	err error,
	probes []ir.ProbeDefinition,
) {
	r.irGenFailureCh <- irGenFailedMessage{
		processID: processID,
		err:       err,
		probes:    probes,
	}
	r.Reporter.ReportIRGenFailed(processID, err, probes)
}
