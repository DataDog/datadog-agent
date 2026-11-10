// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package integrationtests

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	gpuspec "github.com/DataDog/datadog-agent/pkg/collector/corechecks/gpu/spec"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
)

const (
	// systemProbeBinEnv is the path to the system-probe binary. Setting it makes the spec tests run
	// the check against a real system-probe with the GPU eBPF probes.
	systemProbeBinEnv = "GPU_TEST_SYSTEM_PROBE_BIN"
	// systemProbeBPFDirEnv overrides the directory system-probe loads its eBPF objects from.
	systemProbeBPFDirEnv = "GPU_TEST_SYSTEM_PROBE_BPF_DIR"
	// defaultSystemProbeBPFDir is where `dda inv system-probe.build` installs the eBPF objects.
	defaultSystemProbeBPFDir = "/opt/datadog-agent/embedded/share/system-probe/ebpf"

	systemProbeReadyTimeout = 90 * time.Second
	systemProbeStopTimeout  = 30 * time.Second
	systemProbeLogTailLines = 80
)

const systemProbeConfigTemplate = `system_probe_config:
  sysprobe_socket: %[1]s
  bpf_dir: %[2]s
  enable_co_re: true
  enable_runtime_compiler: false
  allow_runtime_compiled_fallback: false
log_file: %[3]s
log_level: debug
discovery:
  enabled: false
gpu_monitoring:
  enabled: true
  enable_ebpf_probes: true
  # Only the eBPF probes are validated, see systemProbeConfigFeatures.
  prm_endpoint_enabled: false
  driver_events_enabled: false
`

// systemProbeDatadogConfig is the core Agent configuration system-probe reads. There is no core
// Agent: cmd_port 0 makes system-probe skip it, otherwise it waits at startup for one on the default
// IPC port and the GPU module does not start.
const systemProbeDatadogConfig = `api_key: '00000000000000000000000000000001'
hostname: gpu-spec-integration-test
cmd_port: 0
`

// startSystemProbeIfRequested starts the system-probe binary named by GPU_TEST_SYSTEM_PROBE_BIN with
// the GPU module and its eBPF probes, as it runs in production, and points the GPU check of this
// process at it. It returns the configuration features to validate the spec with, which are none
// when the variable is unset.
//
// Call it after the GPUs are configured: system-probe holds NVML while it runs, which blocks MIG
// mode changes. It stops when the test ends, before the cleanups registered earlier run.
func startSystemProbeIfRequested(t *testing.T) map[gpuspec.ConfigFeature]bool {
	t.Helper()

	bin := os.Getenv(systemProbeBinEnv)
	if bin == "" {
		t.Logf("%s is not set, the check runs without system-probe", systemProbeBinEnv)
		return nil
	}
	bpfDir := os.Getenv(systemProbeBPFDirEnv)
	if bpfDir == "" {
		bpfDir = defaultSystemProbeBPFDir
	}
	requireSystemProbePrerequisites(t, bin, bpfDir)

	version, err := exec.Command(bin, "version").CombinedOutput()
	require.NoError(t, err, "get system-probe version: %s", version)
	t.Logf("starting system-probe %s: %s (eBPF objects in %s)", bin, strings.TrimSpace(string(version)), bpfDir)

	// Not t.TempDir(): the directory is kept when the test fails, for the system-probe logs.
	dir, err := os.MkdirTemp("", "gpu-spec-system-probe-")
	require.NoError(t, err)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("system-probe output and logs kept in %s", dir)
			return
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Logf("remove %s: %v", dir, err)
		}
	})
	socketPath := filepath.Join(dir, "sysprobe.sock")
	logPath := filepath.Join(dir, "system-probe.log")
	systemProbeConfigPath := filepath.Join(dir, "system-probe.yaml")
	datadogConfigPath := filepath.Join(dir, "datadog.yaml")
	require.NoError(t, os.WriteFile(systemProbeConfigPath, fmt.Appendf(nil, systemProbeConfigTemplate, socketPath, bpfDir, logPath), 0o600))
	require.NoError(t, os.WriteFile(datadogConfigPath, []byte(systemProbeDatadogConfig), 0o600))

	output, err := os.Create(filepath.Join(dir, "system-probe.out"))
	require.NoError(t, err)
	defer output.Close()

	cmd := exec.Command(bin, "run", "--config", systemProbeConfigPath, "--datadogcfgpath", datadogConfigPath)
	cmd.Env = append(os.Environ(),
		// There is no core Agent to stream the configuration from; without this system-probe waits
		// for it before starting.
		"DD_REMOTE_AGENT_CONFIGSTREAM_CONSUMER_ENABLED=false",
	)
	cmd.Stdout = output
	cmd.Stderr = output
	// Kill system-probe if the test binary dies without running its cleanups, so that it does not
	// keep the eBPF probes loaded and NVML held. Linux sends the signal when the OS thread that
	// started the process exits, not the whole test binary; the Go runtime only ends threads locked
	// with runtime.LockOSThread, which this test does not use, so in practice that is the binary.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
	require.NoError(t, cmd.Start(), "start system-probe")

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	t.Cleanup(func() {
		stopSystemProbe(t, cmd, done)
		if t.Failed() {
			logSystemProbeOutput(t, output.Name())
		}
	})

	waitForSystemProbe(t, socketPath, done, output.Name())

	setSystemProbeConfigInTest(t, map[string]any{
		"system_probe_config.sysprobe_socket":  socketPath,
		"gpu_monitoring.enabled":               true,
		"gpu_monitoring.enable_ebpf_probes":    true,
		"gpu_monitoring.prm_endpoint_enabled":  false,
		"gpu_monitoring.driver_events_enabled": false,
	})

	return systemProbeConfigFeatures()
}

// systemProbeConfigFeatures are the features of the system-probe that startSystemProbeIfRequested
// starts. Its configuration enables only these.
func systemProbeConfigFeatures() map[gpuspec.ConfigFeature]bool {
	return map[gpuspec.ConfigFeature]bool{gpuspec.ConfigFeatureSystemProbeEBPF: true}
}

// requireSystemProbePrerequisites fails with the fix when system-probe cannot load the GPU probes.
func requireSystemProbePrerequisites(t *testing.T, bin string, bpfDir string) {
	t.Helper()

	require.Zero(t, os.Geteuid(), "%s needs root to load the eBPF probes; build the test with `dda inv test` and run the test binary with sudo", systemProbeBinEnv)

	info, err := os.Stat(bin)
	require.NoError(t, err, "%s=%s: build it with `dda inv system-probe.build`", systemProbeBinEnv, bin)
	require.False(t, info.IsDir(), "%s=%s is a directory, it must be the system-probe binary", systemProbeBinEnv, bin)

	// system-probe only loads eBPF objects owned by root that only root can write.
	object := filepath.Join(bpfDir, "co-re", "gpu.o")
	info, err = os.Stat(object)
	require.NoError(t, err, "GPU eBPF object not found; `dda inv system-probe.build` installs it in %s, or set %s", defaultSystemProbeBPFDir, systemProbeBPFDirEnv)
	stat := info.Sys().(*syscall.Stat_t)
	require.True(t, stat.Uid == 0 && stat.Gid == 0 && info.Mode().Perm()&0o022 == 0,
		"%s must be owned by root:root and not writable by group or others, system-probe refuses to load it otherwise", object)
}

// waitForSystemProbe waits until the GPU module of system-probe answers with stats.
func waitForSystemProbe(t *testing.T, socketPath string, done <-chan error, outputPath string) {
	t.Helper()

	client := &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}
	defer client.CloseIdleConnections()

	deadline := time.Now().Add(systemProbeReadyTimeout)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-done:
			logSystemProbeOutput(t, outputPath)
			t.Fatalf("system-probe exited before the GPU module was ready: %v", err)
		default:
		}

		status, err := getSystemProbeGPUCheck(t.Context(), client)
		switch {
		case err == nil && status == http.StatusOK:
			t.Logf("system-probe GPU module is ready on %s", socketPath)
			return
		case err == nil && status == http.StatusNotImplemented:
			logSystemProbeOutput(t, outputPath)
			t.Fatalf("system-probe reports the GPU eBPF probes as disabled")
		case err == nil:
			lastErr = fmt.Errorf("GET /gpu/check returned %d", status)
		default:
			lastErr = err
		}
		time.Sleep(500 * time.Millisecond)
	}

	logSystemProbeOutput(t, outputPath)
	t.Fatalf("system-probe GPU module not ready after %v: %v", systemProbeReadyTimeout, lastErr)
}

func getSystemProbeGPUCheck(ctx context.Context, client *http.Client) (int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://sysprobe/gpu/check", nil)
	if err != nil {
		return 0, err
	}
	response, err := client.Do(request)
	if err != nil {
		return 0, err
	}
	defer response.Body.Close()
	return response.StatusCode, nil
}

// stopSystemProbe stops system-probe like its service manager does, and kills it if it does not exit.
func stopSystemProbe(t *testing.T, cmd *exec.Cmd, done <-chan error) {
	t.Helper()

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("stop system-probe: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Logf("system-probe exited: %v", err)
		}
		return
	case <-time.After(systemProbeStopTimeout):
	}

	t.Errorf("system-probe did not exit %v after SIGTERM, killing it", systemProbeStopTimeout)
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
		t.Errorf("kill system-probe: %v", err)
	}
	<-done
}

// setSystemProbeConfigInTest sets system-probe configuration values in this process, where the GPU
// check reads them, and restores the previous values when the test ends.
func setSystemProbeConfigInTest(t *testing.T, values map[string]any) {
	t.Helper()

	cfg := pkgconfigsetup.SystemProbe()
	for key, value := range values {
		previous := cfg.Get(key)
		cfg.SetInTest(key, value)
		t.Cleanup(func() { cfg.SetInTest(key, previous) })
	}
}

func logSystemProbeOutput(t *testing.T, path string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Logf("read system-probe output %s: %v", path, err)
		return
	}
	lines := bytes.Split(bytes.TrimRight(content, "\n"), []byte("\n"))
	if len(lines) > systemProbeLogTailLines {
		lines = lines[len(lines)-systemProbeLogTailLines:]
	}
	t.Logf("last %d lines of the system-probe output:\n%s", len(lines), bytes.Join(lines, []byte("\n")))
}
