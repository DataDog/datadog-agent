// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf && test

package uprobes

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	manager "github.com/DataDog/ebpf-manager"

	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/bytecode"
	"github.com/DataDog/datadog-agent/pkg/ebpf/uprobes/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries"
	sharedlibstestutil "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	usmutils "github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	procutil "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"

	"github.com/DataDog/datadog-agent/pkg/security/utils"
)

// This file provides infrastructure for testing the uprobe attacher in different environments.
// It includes several runner types:
// - AttacherRunner: Interface for running the attacher (SameProcessAttacherRunner, ContainerizedAttacherRunner)
// - AttacherTargetRunner: Interface for running target processes (FmapperRunner, ContainerizedFmapperRunner)
// The main entry point is RunTestAttacher which orchestrates running both the attacher and target.

// ProbeStatus represents the current state of a probe
type ProbeStatus struct {
	ProbeName string
	PID       int
	Path      string
	IsRunning bool
}

// ProbeRequest defines criteria for matching a probe
type ProbeRequest struct {
	ProbeName     string
	PID           int
	Path          string
	PathValidator func(path string) bool
}

// Matches checks if a probe matches the request criteria
func (r *ProbeRequest) Matches(probe *ProbeStatus) bool {
	if r.ProbeName != "" && r.ProbeName != probe.ProbeName {
		return false
	}
	if r.PID != 0 && r.PID != probe.PID {
		return false
	}

	// The attached path might have prefixes (e.g., /host/root/proc/X/...) so we only
	// check that the probe path ends with the requested path
	if r.Path != "" && !strings.HasSuffix(probe.Path, r.Path) {
		return false
	}
	if r.PathValidator != nil && !r.PathValidator(probe.Path) {
		return false
	}
	return true
}

// AttacherRunner defines the interface for running the uprobe attacher
type AttacherRunner interface {
	// RunAttacher runs the attacher with the given attacher configuration
	RunAttacher(t *testing.T, configName AttacherTestConfigName)

	// GetProbes returns the current state of all attached probes
	GetProbes(t assert.TestingT) []ProbeStatus
}

// AttacherTargetRunner defines the interface for running target processes to which the attacher will attach
type AttacherTargetRunner interface {
	// Run runs the target process with the given paths to open
	Run(t *testing.T, paths ...string)

	// GetTargetPid returns the PID of the target process
	GetTargetPid(t *testing.T) int

	// Stop terminates the target process. This function should not return until the target process has been terminated.
	Stop(t *testing.T)
}

// AttacherTestConfigName is the name of a test configuration for the attacher
// These are defined here, so that attachers running in separate processes can use the same configurations, avoiding drift.
type AttacherTestConfigName string

const (
	// LibraryAndMainAttacherTestConfigName is the name of the test configuration for the attacher that attaches to a libssl library and the main executable
	LibraryAndMainAttacherTestConfigName AttacherTestConfigName = "library-and-main"
)

// AttacherTestConfigs is a map of attacher test config names to attacher configs.
// It's initialized by loadAttacherTestConfigs.
var AttacherTestConfigs map[AttacherTestConfigName]AttacherConfig

// RunTestAttacherConfig is the configuration for a test that runs both the attacher and target process, verifying probe attachment and detachment
type RunTestAttacherConfig struct {
	// WaitTimeForAttach is the time to wait for the probes to be attached
	WaitTimeForAttach time.Duration
	// WaitTimeForDetach is the time to wait for the probes to be detached
	WaitTimeForDetach time.Duration
	// ConfigName is the name of the test configuration to use
	ConfigName AttacherTestConfigName
	// PathsToOpen is the list of paths to open on the target process
	PathsToOpen []string
	// ExpectedProbes is the list of probes that are expected to be attached
	ExpectedProbes []ProbeRequest
}

// RunTestAttacher orchestrates running both the attacher and target process, verifying probe attachment and detachment
func RunTestAttacher(t *testing.T, configName AttacherTestConfigName, attacherRunner AttacherRunner, targetRunner AttacherTargetRunner, testConfig RunTestAttacherConfig) {
	// Run the attacher first, then the target
	attacherRunner.RunAttacher(t, configName)
	targetRunner.Run(t, testConfig.PathsToOpen...)

	// Apply the PID of the target to the wanted probes
	for i := range testConfig.ExpectedProbes {
		testConfig.ExpectedProbes[i].PID = targetRunner.GetTargetPid(t)
	}

	// Wait for the probes to be attached
	var probes []ProbeStatus
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		probes = attacherRunner.GetProbes(t)

		for _, wanted := range testConfig.ExpectedProbes {
			found := false
			for _, probe := range probes {
				if wanted.Matches(&probe) {
					assert.True(c, probe.IsRunning, "matching probe %v should be running, but it's not", probe)
					found = true
					break
				}
			}
			assert.True(c, found, "probe %v not found", wanted)
		}
	}, testConfig.WaitTimeForAttach, 50*time.Millisecond, "did not find all wanted probes %v, got %v", testConfig.ExpectedProbes, probes)

	// At this point everything is attached, so we can stop the target and check the probes get detached
	targetRunner.Stop(t)

	// Wait for the probes to be detached
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		probes = attacherRunner.GetProbes(t)
		for _, wanted := range testConfig.ExpectedProbes {
			found := false
			for _, probe := range probes {
				if wanted.Matches(&probe) {
					assert.False(c, probe.IsRunning, "matching probe %v should be detached, but it's not", probe)
					found = true
					break
				}
			}
			assert.True(c, found, "probe %v not found", wanted)
		}
	}, testConfig.WaitTimeForDetach, 50*time.Millisecond, "did not find all wanted probes %v, got %v", testConfig.ExpectedProbes, probes)
}

// ContainerizedFmapperRunner runs the target process in a container
type ContainerizedFmapperRunner struct {
	pid           int64
	containerName string
}

// NewContainerizedFmapperRunner creates a new runner that executes the target in a container
func NewContainerizedFmapperRunner() AttacherTargetRunner {
	return &ContainerizedFmapperRunner{}
}

// Run starts the target process in a container
func (r *ContainerizedFmapperRunner) Run(t *testing.T, paths ...string) {
	programExecutable := sharedlibstestutil.BuildFmapper(t)
	executableAbsPathInContainer := "/" + filepath.Base(programExecutable)
	mounts := map[string]string{
		programExecutable: executableAbsPathInContainer,
	}

	for _, path := range paths {
		// Mount the directory containing the shared library, not the library itself, so that
		// we don't open it and the attacher doesn't attach to the process that opens it before
		// the test process starts.
		mounts[filepath.Dir(path)] = filepath.Dir(path)
	}

	r.containerName = fmt.Sprintf("fmapper-testutil-%s", utils.RandString(10))
	scanner := sharedlibstestutil.BuildFmapperScanner(t)
	dockerConfig := dockerutils.NewRunConfig(
		dockerutils.NewBaseConfig(
			r.containerName,
			scanner,
			dockerutils.WithTimeout(5*time.Second),
		),
		dockerutils.MinimalDockerImage,
		executableAbsPathInContainer,
		dockerutils.WithBinaryArgs(paths),
		dockerutils.WithMounts(mounts),
	)

	require.NoError(t, dockerutils.Run(t, dockerConfig))

	// It might take a while for the container to appear in the list of containers
	// even if it has already started.
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var err error
		r.pid, err = dockerutils.GetMainPID(r.containerName)
		assert.NoError(c, err, "failed to get docker PID")
	}, 3*time.Second, 10*time.Millisecond, "expected to get docker PID")
}

// GetTargetPid returns the PID of the target process
func (r *ContainerizedFmapperRunner) GetTargetPid(_ *testing.T) int {
	return int(r.pid)
}

// Stop terminates the target container
func (r *ContainerizedFmapperRunner) Stop(t *testing.T) {
	err := exec.Command("docker", "rm", "-f", r.containerName).Run()
	var exitErr *exec.ExitError
	stderr := ""
	if errors.As(err, &exitErr) {
		stderr = string(exitErr.Stderr)
	}
	assert.NoError(t, err, "failed to kill container: %s", stderr)
}

// FmapperRunner runs the target process directly on the host
type FmapperRunner struct {
	cmd *exec.Cmd
}

// NewFmapperRunner creates a new runner that executes the target directly on the host
func NewFmapperRunner() AttacherTargetRunner {
	return &FmapperRunner{}
}

// Run starts the target process on the host
func (r *FmapperRunner) Run(t *testing.T, paths ...string) {
	programExecutable := sharedlibstestutil.BuildFmapper(t)

	var err error
	r.cmd, err = sharedlibstestutil.OpenFromProcess(t, programExecutable, paths...)
	require.NoError(t, err)
}

// GetTargetPid returns the PID of the target process
func (r *FmapperRunner) GetTargetPid(_ *testing.T) int {
	return r.cmd.Process.Pid
}

// Stop terminates the target process
func (r *FmapperRunner) Stop(t *testing.T) {
	err := r.cmd.Process.Kill()
	require.NoError(t, err, "failed to kill fmapper")
}

// SameProcessAttacherRunner runs the attacher in the same process as the caller code
type SameProcessAttacherRunner struct {
	attachedProbes []attachedProbe
}

type attachedProbe struct {
	probe *manager.Probe
	fpath *usmutils.FilePath
}

var _ AttacherRunner = &SameProcessAttacherRunner{}

// NewSameProcessAttacherRunner creates a new runner that executes the attacher in the same process as the caller code
func NewSameProcessAttacherRunner() AttacherRunner {
	return &SameProcessAttacherRunner{}
}

func (r *SameProcessAttacherRunner) onAttach(probe *manager.Probe, fpath *usmutils.FilePath) {
	r.attachedProbes = append(r.attachedProbes, attachedProbe{probe: probe, fpath: fpath})
}

// RunAttacher starts the attacher in the same process as the test
func (r *SameProcessAttacherRunner) RunAttacher(t *testing.T, configName AttacherTestConfigName) {
	mgr := manager.Manager{}
	procMon := launchProcessMonitor(t, false)

	cfg := GetAttacherTestConfig(t, configName)

	attacher, err := NewUprobeAttacher("test", "test", cfg, &mgr, r.onAttach, &NativeBinaryInspector{}, procMon)
	require.NoError(t, err)
	require.NotNil(t, attacher)

	err = ddebpf.LoadCOREAsset("uprobe_attacher-test.o", func(buf bytecode.AssetReader, opts manager.Options) error {
		require.NoError(t, mgr.InitWithOptions(buf, opts))
		require.NoError(t, mgr.Start())
		t.Cleanup(func() { _ = mgr.Stop(manager.CleanAll) })

		return nil
	})
	require.NoError(t, err)

	require.NoError(t, attacher.Start())
	t.Cleanup(attacher.Stop)
}

// GetProbes returns the current state of all attached probes
func (r *SameProcessAttacherRunner) GetProbes(_ assert.TestingT) []ProbeStatus {
	probes := []ProbeStatus{}
	for _, probe := range r.attachedProbes {
		probes = append(probes, ProbeStatus{
			ProbeName: probe.probe.EBPFFuncName,
			PID:       int(probe.fpath.PID),
			Path:      probe.fpath.HostPath,
			IsRunning: probe.probe.IsRunning(),
		})
	}
	return probes
}

// ContainerizedAttacherRunner runs the attacher in a container
type ContainerizedAttacherRunner struct {
	containerName  string
	probesEndpoint string
}

// NewContainerizedAttacherRunner creates a new runner that executes the attacher in a container
func NewContainerizedAttacherRunner() AttacherRunner {
	return &ContainerizedAttacherRunner{
		probesEndpoint: "http://localhost:8080/probes",
	}
}

// RunAttacher starts the attacher in a container
func (r *ContainerizedAttacherRunner) RunAttacher(t *testing.T, configName AttacherTestConfigName) {
	r.containerName = fmt.Sprintf("uprobe-attacher-testutil-%s", utils.RandString(10))
	attacherBin := testutil.BuildStandaloneAttacher(t)

	// Get the ebpf config to ensure we have the same paths and config
	ebpfCfg := ddebpf.NewConfig()
	require.NotNil(t, ebpfCfg)

	mounts := map[string]string{
		attacherBin:                      attacherBin,
		ebpfCfg.BPFDir:                   ebpfCfg.BPFDir,
		ebpfCfg.BTFPath:                  ebpfCfg.BTFPath,
		ebpfCfg.KernelHeadersDownloadDir: ebpfCfg.KernelHeadersDownloadDir,
		ebpfCfg.RuntimeCompilerOutputDir: ebpfCfg.RuntimeCompilerOutputDir,
		ebpfCfg.BTFOutputDir:             ebpfCfg.BTFOutputDir,
		"/etc/os-release":                "/host/etc/os-release", // for correct BTF detection
		"/sys":                           "/sys",
		"/proc":                          "/host/proc",
		"/":                              "/host/root",
	}

	envVarsMap := map[string]string{
		"HOST_ROOT":                          "/host/root",
		"HOST_PROC":                          "/host/proc",
		"HOST_ETC":                           "/host/etc", // Allow the attacher to read /etc/os-release
		"DOCKER_DD_AGENT":                    "true",      // This env-var comes from the agent docker image, simulate it here
		"DD_SYSTEM_PROBE_BPF_DIR":            ebpfCfg.BPFDir,
		"DD_SYSTEM_PROBE_BTF_PATH":           ebpfCfg.BTFPath,
		"DD_SYSTEM_PROBE_KERNEL_HEADERS_DIR": ebpfCfg.KernelHeadersDownloadDir,
		"DD_SYSTEM_PROBE_RUNTIME_COMPILER_OUTPUT_DIR": ebpfCfg.RuntimeCompilerOutputDir,
		"DD_SYSTEM_PROBE_BTF_OUTPUT_DIR":              ebpfCfg.BTFOutputDir,
	}

	scanner, err := procutil.NewScanner(regexp.MustCompile("standalone attacher ready to serve requests"), nil)
	require.NoError(t, err)

	envVars := []string{}
	for k, v := range envVarsMap {
		envVars = append(envVars, fmt.Sprintf("%s=%s", k, v))
	}

	dockerConfig := dockerutils.NewRunConfig(
		dockerutils.NewBaseConfig(
			r.containerName,
			scanner,
			dockerutils.WithTimeout(30*time.Second),
			dockerutils.WithRetries(2),
			dockerutils.WithEnv(envVars),
		),
		dockerutils.MinimalDockerImage,
		attacherBin,
		dockerutils.WithBinaryArgs(
			[]string{
				"-test.v", // Ensure the test framework doesn't suppress output
				"-config", string(configName),
			},
		),
		dockerutils.WithMounts(mounts),
		dockerutils.WithPrivileged(true),
		dockerutils.WithNetworkMode("host"),
		dockerutils.WithPIDMode("host"),
	)

	require.NoError(t, dockerutils.Run(t, dockerConfig))

	// Wait for HTTP server to start
	require.Eventually(t, func() bool {
		resp, err := http.Get(r.probesEndpoint)
		if err == nil {
			defer resp.Body.Close()
		}
		return err == nil && resp.StatusCode == http.StatusOK
	}, 5*time.Second, 100*time.Millisecond, "HTTP server is not ready")
}

// GetProbes returns the current state of all attached probes via HTTP
func (r *ContainerizedAttacherRunner) GetProbes(t assert.TestingT) []ProbeStatus {
	resp, err := http.Get(r.probesEndpoint)
	assert.NoError(t, err)
	defer resp.Body.Close()

	var probes []ProbeStatus
	err = json.NewDecoder(resp.Body).Decode(&probes)
	assert.NoError(t, err)

	return probes
}

// loadAttacherTestConfigs loads the test configurations for the attacher
func loadAttacherTestConfigs() {
	if AttacherTestConfigs != nil {
		return // Already loaded
	}

	AttacherTestConfigs = make(map[AttacherTestConfigName]AttacherConfig)
	AttacherTestConfigs[LibraryAndMainAttacherTestConfigName] = AttacherConfig{
		Rules: []*AttachRule{
			{
				LibraryNameRegex: regexp.MustCompile(`libssl.so`),
				Targets:          AttachToSharedLibraries,
				ProbesSelector: []manager.ProbesSelector{
					&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__SSL_connect"}},
				},
			},
			{
				Targets: AttachToExecutable,
				ProbesSelector: []manager.ProbesSelector{
					&manager.ProbeSelector{ProbeIdentificationPair: manager.ProbeIdentificationPair{EBPFFuncName: "uprobe__main"}},
				},
				ProbeOptionsOverride: map[string]ProbeOptions{
					"uprobe__main": {
						IsManualReturn: false,
						Symbol:         "main.main",
					},
				},
			},
		},
		ExcludeTargets:        ExcludeInternal | ExcludeSelf,
		EnableDetailedLogging: true,
		SharedLibsLibsets:     []sharedlibraries.Libset{sharedlibraries.LibsetCrypto},
	}
}

// GetAttacherTestConfig returns the test configuration for the specified config name
func GetAttacherTestConfig(t *testing.T, configName AttacherTestConfigName) AttacherConfig {
	loadAttacherTestConfigs()
	require.Contains(t, AttacherTestConfigs, configName, "config not found")
	cfg := AttacherTestConfigs[configName]

	ebpfCfg := ddebpf.NewConfig()
	require.NotNil(t, ebpfCfg)

	cfg.EbpfConfig = ebpfCfg

	return cfg
}
