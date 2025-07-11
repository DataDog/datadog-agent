// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// This doesn't need BPF, but it's built with this tag to only run with
// system-probe tests.
//go:build test && linux_bpf

package module

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	gorillamux "github.com/gorilla/mux"
	"github.com/prometheus/procfs"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"
	"go.uber.org/fx"
	"golang.org/x/sys/unix"

	compcore "github.com/DataDog/datadog-agent/comp/core"
	taggerfxmock "github.com/DataDog/datadog-agent/comp/core/tagger/fx-mock"
	taggermock "github.com/DataDog/datadog-agent/comp/core/tagger/mock"
	"github.com/DataDog/datadog-agent/comp/core/tagger/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/core"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/tls/nodejs"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	globalutils "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

var mockedTime = time.Date(2024, 12, 1, 12, 12, 12, 2, time.UTC)

func findService(pid int, services []model.Service) *model.Service {
	for _, s := range services {
		if s.PID == pid {
			return &s
		}
	}

	return nil
}

type testDiscoveryModule struct {
	url              string
	mockWmeta        workloadmetamock.Mock
	mockTagger       taggermock.Mock
	mockTimeProvider *MocktimeProvider
}

// setProcessContainer creates mock process and container entities in workloadmeta for testing.
func (m *testDiscoveryModule) setProcessContainer(pid int, containerID string, collectorTags []string, taggerTags []string) {
	// Create a container entity
	container := &workloadmeta.Container{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindContainer,
			ID:   containerID,
		},
		PID:           pid,
		CollectorTags: collectorTags,
		State:         workloadmeta.ContainerState{Running: true},
	}
	m.mockWmeta.Set(container)

	// Set tagger tags as high cardinality tags
	m.mockTagger.SetTags(types.NewEntityID(types.ContainerID, containerID), "fake", nil, nil, taggerTags, nil)
}

func setupDiscoveryModuleWithNetwork(t *testing.T, getNetworkCollector networkCollectorFactory) *testDiscoveryModule {
	t.Helper()
	mockCtrl := gomock.NewController(t)

	mockWmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		compcore.MockBundle(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	mockTagger := taggerfxmock.SetupFakeTagger(t)
	mTimeProvider := NewMocktimeProvider(mockCtrl)

	mux := gorillamux.NewRouter()

	discovery := newDiscoveryWithNetwork(mockWmeta, mockTagger, mTimeProvider, getNetworkCollector)
	discovery.config.CPUUsageUpdateDelay = time.Second
	discovery.config.NetworkStatsPeriod = time.Second
	discovery.Register(module.NewRouter(string(config.DiscoveryModule), mux))
	t.Cleanup(discovery.Close)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &testDiscoveryModule{
		url:              srv.URL,
		mockWmeta:        mockWmeta,
		mockTagger:       mockTagger,
		mockTimeProvider: mTimeProvider,
	}
}

func setupDiscoveryModule(t *testing.T) *testDiscoveryModule {
	t.Helper()
	return setupDiscoveryModuleWithNetwork(t, newNetworkCollector)
}

// makeRequest wraps the request to the discovery module, setting the JSON body if provided,
// and returning the response as the given type.
func makeRequest[T any](t require.TestingT, url string, params *core.Params) *T {
	var body *bytes.Buffer
	if params != nil {
		jsonData, err := params.ToJSON()
		require.NoError(t, err, "failed to serialize params to JSON")
		body = bytes.NewBuffer(jsonData)
	}

	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(http.MethodGet, url, body)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(http.MethodGet, url, nil)
	}
	require.NoError(t, err, "failed to create request")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err, "failed to send request")
	defer resp.Body.Close()

	res := new(T)
	err = json.NewDecoder(resp.Body).Decode(res)
	require.NoError(t, err, "failed to decode response")

	return res
}

// getRunningPids wraps the process.Pids function, returning a slice of ints
// that can be used as the pids query param.
func getRunningPids(t require.TestingT) []int {
	pids, err := process.Pids()
	require.NoError(t, err)

	pidsInt := make([]int, len(pids))
	for i, v := range pids {
		pidsInt[i] = int(v)
	}

	return pidsInt
}

// getCheckWithParams call the /discovery/check endpoint with the given params.
func getCheckWithParams(t require.TestingT, url string, params *core.Params) *model.ServicesResponse {
	location := url + "/" + string(config.DiscoveryModule) + pathCheck
	return makeRequest[model.ServicesResponse](t, location, params)
}

// TODO: remove this after refactor
func getCheckServices(t require.TestingT, url string) *model.ServicesResponse {
	return getCheckWithParams(t, url, nil)
}

func startTCPServer(t *testing.T, proto string, address string) (*os.File, *net.TCPAddr) {
	listener, err := net.Listen(proto, address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	tcpAddr := listener.Addr().(*net.TCPAddr)

	f, err := listener.(*net.TCPListener).File()
	defer listener.Close()
	require.NoError(t, err)

	return f, tcpAddr
}

func startTCPClient(t *testing.T, proto string, server *net.TCPAddr) (*os.File, *net.TCPAddr) {
	client, err := net.DialTCP(proto, nil, server)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	f, err := client.File()
	defer client.Close()
	require.NoError(t, err)

	return f, client.LocalAddr().(*net.TCPAddr)
}

func startUDPServer(t *testing.T, proto string, address string) (*os.File, *net.UDPAddr) {
	lnPacket, err := net.ListenPacket(proto, address)
	require.NoError(t, err)
	t.Cleanup(func() { _ = lnPacket.Close() })

	f, err := lnPacket.(*net.UDPConn).File()
	defer lnPacket.Close()
	require.NoError(t, err)

	return f, lnPacket.LocalAddr().(*net.UDPAddr)
}

func startUDPClient(t *testing.T, proto string, server *net.UDPAddr) (*os.File, *net.UDPAddr) {
	udpClient, err := net.DialUDP(proto, nil, server)
	require.NoError(t, err)
	t.Cleanup(func() { _ = udpClient.Close() })

	f, err := udpClient.File()
	defer udpClient.Close()
	require.NoError(t, err)

	return f, udpClient.LocalAddr().(*net.UDPAddr)
}

func disableCloseOnExec(t *testing.T, f *os.File) {
	_, _, syserr := syscall.Syscall(syscall.SYS_FCNTL, f.Fd(), syscall.F_SETFD, 0)
	require.Equal(t, syscall.Errno(0x0), syserr)
}

func startProcessWithFile(t *testing.T, f *os.File) *exec.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	// Disable close-on-exec so that the process gets it
	t.Cleanup(func() { f.Close() })
	disableCloseOnExec(t, f)

	cmd := exec.CommandContext(ctx, "sleep", "1000")
	err := cmd.Start()
	require.NoError(t, err)
	f.Close()

	return cmd
}

// Check that we get (only) listening processes for all expected protocols.
func TestBasic(t *testing.T) {
	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

	var expectedPIDs []int
	var unexpectedPIDs []int
	expectedPorts := make(map[int]int)

	startTCP := func(proto string) {
		f, server := startTCPServer(t, proto, "")
		cmd := startProcessWithFile(t, f)
		expectedPIDs = append(expectedPIDs, cmd.Process.Pid)
		expectedPorts[cmd.Process.Pid] = server.Port

		f, _ = startTCPClient(t, proto, server)
		cmd = startProcessWithFile(t, f)
		unexpectedPIDs = append(unexpectedPIDs, cmd.Process.Pid)
	}

	startUDP := func(proto string) {
		f, server := startUDPServer(t, proto, ":8083")
		cmd := startProcessWithFile(t, f)
		expectedPIDs = append(expectedPIDs, cmd.Process.Pid)
		expectedPorts[cmd.Process.Pid] = server.Port

		f, _ = startUDPClient(t, proto, server)
		cmd = startProcessWithFile(t, f)
		unexpectedPIDs = append(unexpectedPIDs, cmd.Process.Pid)
	}

	startTCP("tcp4")
	startTCP("tcp6")
	startUDP("udp4")
	startUDP("udp6")

	seen := make(map[int]model.Service)
	// Eventually to give the processes time to start
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getCheckServices(collect, discovery.url)
		for _, s := range resp.StartedServices {
			seen[s.PID] = s
		}

		for _, pid := range expectedPIDs {
			require.Contains(collect, seen, pid)
			require.Contains(collect, seen[pid].Ports, uint16(expectedPorts[pid]))
			require.Equal(collect, seen[pid].LastHeartbeat, mockedTime.Unix())
			assertStat(collect, seen[pid])
		}
		for _, pid := range unexpectedPIDs {
			assert.NotContains(collect, seen, pid)
		}
	}, 30*time.Second, 100*time.Millisecond)
}

// Check that we get all listening ports for a process
func TestPorts(t *testing.T) {
	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

	var expectedPorts []uint16
	var unexpectedPorts []uint16

	startTCP := func(proto string) {
		serverf, server := startTCPServer(t, proto, "")
		t.Cleanup(func() { serverf.Close() })
		clientf, client := startTCPClient(t, proto, server)
		t.Cleanup(func() { clientf.Close() })

		expectedPorts = append(expectedPorts, uint16(server.Port))
		unexpectedPorts = append(unexpectedPorts, uint16(client.Port))
	}

	startUDP := func(proto string) {
		serverf, server := startUDPServer(t, proto, ":8083")
		t.Cleanup(func() { _ = serverf.Close() })
		clientf, client := startUDPClient(t, proto, server)
		t.Cleanup(func() { clientf.Close() })

		expectedPorts = append(expectedPorts, uint16(server.Port))
		unexpectedPorts = append(unexpectedPorts, uint16(client.Port))

		ephemeralf, ephemeral := startUDPServer(t, proto, "")
		t.Cleanup(func() { _ = ephemeralf.Close() })
		unexpectedPorts = append(unexpectedPorts, uint16(ephemeral.Port))
	}

	startTCP("tcp4")
	startTCP("tcp6")
	startUDP("udp4")
	startUDP("udp6")

	// Create a log file for the current process to test log file collection
	tempDir, err := os.MkdirTemp("", "test-log-files")
	require.NoError(t, err)
	t.Cleanup(func() { os.RemoveAll(tempDir) })

	logFilePath := filepath.Join(tempDir, "test.log")
	logFile, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	require.NoError(t, err)
	t.Cleanup(func() { logFile.Close() })

	expectedPortsMap := make(map[uint16]struct{}, len(expectedPorts))

	pid := os.Getpid()
	// First call will not return anything, as all services will be potentials.
	_ = getCheckServices(t, discovery.url)
	resp := getCheckServices(t, discovery.url)
	startEvent := findService(pid, resp.StartedServices)
	require.NotNilf(t, startEvent, "could not find start event for pid %v", pid)

	for _, port := range expectedPorts {
		expectedPortsMap[port] = struct{}{}
		assert.Contains(t, startEvent.Ports, port)
	}
	for _, port := range unexpectedPorts {
		// An unexpected port number can also be expected since UDP and TCP and
		// v4 and v6 are all in the same list. Just skip the extra check in that
		// case since it should be rare.
		if _, alsoExpected := expectedPortsMap[port]; alsoExpected {
			continue
		}

		// Do not assert about this since this check can spuriously fail since
		// the test infrastructure opens a listening TCP socket on an ephimeral
		// port, and since we mix the different protocols we could find that on
		// the unexpected port list.
		if slices.Contains(startEvent.Ports, port) {
			t.Logf("unexpected port %v also found", port)
		}
	}

	// Check that log files are collected
	assert.Contains(t, startEvent.LogFiles, logFilePath,
		"Process %d should have log file %s", pid, logFilePath)
}

func TestPortsLimits(t *testing.T) {
	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

	var expectedPorts []int

	openPort := func(address string) {
		serverf, server := startTCPServer(t, "tcp4", address)
		t.Cleanup(func() { serverf.Close() })

		expectedPorts = append(expectedPorts, server.Port)
	}

	openPort("127.0.0.1:8081")

	for i := 0; i < maxNumberOfPorts; i++ {
		openPort("")
	}

	openPort("127.0.0.1:8082")

	slices.Sort(expectedPorts)

	pid := os.Getpid()

	// Firt call will not return anything, as all services will be potentials.
	_ = getCheckServices(t, discovery.url)
	resp := getCheckServices(t, discovery.url)
	startEvent := findService(pid, resp.StartedServices)
	require.NotNilf(t, startEvent, "could not find start event for pid %v", pid)

	assert.Contains(t, startEvent.Ports, uint16(8081))
	assert.Contains(t, startEvent.Ports, uint16(8082))
	assert.Len(t, startEvent.Ports, maxNumberOfPorts)
	for i := 0; i < maxNumberOfPorts-2; i++ {
		assert.Contains(t, startEvent.Ports, uint16(expectedPorts[i]))
	}
}

func TestServiceName(t *testing.T) {
	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

	trMeta := tracermetadata.TracerMetadata{
		SchemaVersion:  1,
		RuntimeID:      "test-runtime-id",
		TracerLanguage: "go",
		ServiceName:    "test-service",
	}
	data, err := trMeta.MarshalMsg(nil)
	require.NoError(t, err)

	createTracerMemfd(t, data)

	listener, err := net.Listen("tcp", "")
	require.NoError(t, err)
	f, err := listener.(*net.TCPListener).File()
	listener.Close()

	// Disable close-on-exec so that the sleep gets it
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	disableCloseOnExec(t, f)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	cmd := exec.CommandContext(ctx, "sleep", "1000")
	cmd.Dir = "/tmp/"
	cmd.Env = append(cmd.Env, "OTHER_ENV=test")
	cmd.Env = append(cmd.Env, "DD_SERVICE=foo😀bar")
	cmd.Env = append(cmd.Env, "YET_OTHER_ENV=test")
	err = cmd.Start()
	require.NoError(t, err)
	f.Close()

	pid := cmd.Process.Pid
	var startEvent *model.Service
	// Eventually to give the processes time to start
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getCheckServices(collect, discovery.url)
		startEvent = findService(pid, resp.StartedServices)
		require.NotNilf(collect, startEvent, "could not find start event for pid %v", pid)

		// Non-ASCII character removed due to normalization.
		assert.Equal(collect, "foo_bar", startEvent.DDService)
		assert.Equal(collect, "sleep", startEvent.GeneratedName)
		assert.Equal(collect, string(usm.CommandLine), startEvent.GeneratedNameSource)
		assert.False(collect, startEvent.DDServiceInjected)
		assert.Equal(collect, startEvent.ContainerID, "")
		assert.Equal(collect, startEvent.LastHeartbeat, mockedTime.Unix())
	}, 30*time.Second, 100*time.Millisecond)

	// Verify tracer metadata
	assert.Equal(t, []tracermetadata.TracerMetadata{trMeta}, startEvent.TracerMetadata)
	assert.Equal(t, string(language.Go), startEvent.Language)
}

func TestServiceLifetime(t *testing.T) {
	startService := func() (*exec.Cmd, context.CancelFunc) {
		listener, err := net.Listen("tcp", "")
		require.NoError(t, err)
		f, err := listener.(*net.TCPListener).File()
		listener.Close()

		// Disable close-on-exec so that the sleep gets it
		require.NoError(t, err)
		t.Cleanup(func() { f.Close() })
		disableCloseOnExec(t, f)

		ctx, cancel := context.WithCancel(context.Background())
		t.Cleanup(func() { cancel() })

		cmd := exec.CommandContext(ctx, "sleep", "1000")
		cmd.Dir = "/tmp/"
		cmd.Env = append(cmd.Env, "DD_SERVICE=foo_bar")
		err = cmd.Start()
		require.NoError(t, err)
		f.Close()

		return cmd, cancel
	}

	checkService := func(t assert.TestingT, service *model.Service, expectedTime time.Time) {
		// Non-ASCII character removed due to normalization.
		assert.Equal(t, "foo_bar", service.DDService)
		assert.Equal(t, "sleep", service.GeneratedName)
		assert.Equal(t, string(usm.CommandLine), service.GeneratedNameSource)
		assert.False(t, service.DDServiceInjected)
		assert.Equal(t, service.ContainerID, "")
		assert.Equal(t, service.LastHeartbeat, expectedTime.Unix())
	}

	stopService := func(cmd *exec.Cmd, cancel context.CancelFunc) {
		cancel()
		_ = cmd.Wait()
	}

	t.Run("stop", func(t *testing.T) {
		discovery := setupDiscoveryModule(t)
		discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

		// Start the service and check we found it.
		cmd, cancel := startService()
		pid := cmd.Process.Pid
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			resp := getCheckServices(collect, discovery.url)
			startEvent := findService(pid, resp.StartedServices)
			require.NotNilf(collect, startEvent, "could not find start event for pid %v", pid)
			checkService(collect, startEvent, mockedTime)
		}, 30*time.Second, 100*time.Millisecond)

		// Stop the service, and look for the stop event.
		stopService(cmd, cancel)
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			resp := getCheckServices(collect, discovery.url)
			stopEvent := findService(pid, resp.StoppedServices)
			t.Logf("stopped service: %+v", resp.StoppedServices)
			require.NotNilf(collect, stopEvent, "could not find stop event for pid %v", pid)
			checkService(collect, stopEvent, mockedTime)
		}, 30*time.Second, 100*time.Millisecond)
	})

	t.Run("heartbeat", func(t *testing.T) {
		discovery := setupDiscoveryModule(t)

		startEventSeen := false

		discovery.mockTimeProvider.EXPECT().Now().DoAndReturn(func() time.Time {
			if !startEventSeen {
				return mockedTime
			}

			return mockedTime.Add(core.HeartbeatTime)
		}).AnyTimes()

		cmd, cancel := startService()
		t.Cleanup(cancel)

		pid := cmd.Process.Pid
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			resp := getCheckServices(collect, discovery.url)
			startEvent := findService(pid, resp.StartedServices)
			require.NotNilf(collect, startEvent, "could not find start event for pid %v", pid)
			checkService(collect, startEvent, mockedTime)
		}, 30*time.Second, 100*time.Millisecond)

		startEventSeen = true
		resp := getCheckServices(t, discovery.url)
		heartbeatEvent := findService(pid, resp.HeartbeatServices)
		require.NotNilf(t, heartbeatEvent, "could not find heartbeat event for pid %v", pid)
		checkService(t, heartbeatEvent, mockedTime.Add(core.HeartbeatTime))
	})
}

func makeAlias(t *testing.T, alias string, serverBin string) string {
	binDir := filepath.Dir(serverBin)
	aliasPath := filepath.Join(binDir, alias)

	target, err := os.Readlink(aliasPath)
	if err == nil && target == serverBin {
		return aliasPath
	}

	os.Remove(aliasPath)
	err = os.Symlink(serverBin, aliasPath)
	require.NoError(t, err)

	return aliasPath
}

func buildFakeServer(t *testing.T) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	serverBin, err := usmtestutil.BuildGoBinaryWrapper(filepath.Join(curDir, "testutil"), "fake_server")
	require.NoError(t, err)

	for _, alias := range []string{"java", "node", "sshd", "dotnet"} {
		makeAlias(t, alias, serverBin)
	}

	return filepath.Dir(serverBin)
}

func TestPythonFromBashScript(t *testing.T) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	pythonScriptPath := filepath.Join(curDir, "testdata", "script.py")

	t.Run("PythonFromBashScript", func(t *testing.T) {
		testCaptureWrappedCommands(t, pythonScriptPath, []string{"sh", "-c"}, func(service model.Service) bool {
			return service.Language == string(language.Python)
		})
	})
	t.Run("DirectPythonScript", func(t *testing.T) {
		testCaptureWrappedCommands(t, pythonScriptPath, nil, func(service model.Service) bool {
			return service.Language == string(language.Python)
		})
	})
}

func testCaptureWrappedCommands(t *testing.T, script string, commandWrapper []string, validator func(service model.Service) bool) {
	// Changing permissions
	require.NoError(t, os.Chmod(script, 0o755))

	commandLineArgs := append(commandWrapper, script)
	cmd := exec.Command(commandLineArgs[0], commandLineArgs[1:]...)
	// Running the binary in the background
	require.NoError(t, cmd.Start())

	var proc *process.Process
	var err error
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		proc, err = process.NewProcess(int32(cmd.Process.Pid))
		require.NoError(collect, err)

		// If we wrap the script with `sh -c`, we can have differences between a
		// local run and a kmt run, as for kmt `sh` is symbolic link to bash,
		// while locally it can be a symbolic link to dash.
		//
		// In the dash case, we will see 2 processes `sh -c script.py` and a
		// sub-process `python3 script.py`, while in the bash case we will see
		// only `python3 script.py` (after initially potentially seeing `sh -c
		// script.py` before the process execs). We need to check for the
		// command line arguments of the process to make sure we are looking at
		// the right process.
		cmdline, err := proc.Cmdline()
		require.NoError(t, err)
		if cmdline == strings.Join(commandLineArgs, " ") && len(commandWrapper) > 0 {
			var children []*process.Process
			children, err = proc.Children()
			require.NoError(collect, err)
			require.Len(collect, children, 1)
			proc = children[0]
		}
	}, 10*time.Second, 100*time.Millisecond)

	t.Cleanup(func() { _ = proc.Kill() })

	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

	pid := int(proc.Pid)
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getCheckServices(collect, discovery.url)
		startEvent := findService(pid, resp.StartedServices)
		require.NotNilf(collect, startEvent, "could not find start event for pid %v", pid)
		assert.True(collect, validator(*startEvent))
	}, 30*time.Second, 100*time.Millisecond)
}

func TestAPMInstrumentationProvided(t *testing.T) {
	curDir, err := testutil.CurDir()
	assert.NoError(t, err)

	testCases := map[string]struct {
		commandline []string // The command line of the fake server
		language    language.Language
		env         []string
	}{
		"dotnet": {
			commandline: []string{"dotnet", "foo.dll"},
			language:    language.DotNet,
			env: []string{
				"CORECLR_ENABLE_PROFILING=1",
			},
		},
		"java - dd-java-agent.jar": {
			commandline: []string{"java", "-javaagent:/path/to/dd-java-agent.jar", "-jar", "foo.jar"},
			language:    language.Java,
		},
		"java - datadog.jar": {
			commandline: []string{"java", "-javaagent:/path/to/datadog-java-agent.jar", "-jar", "foo.jar"},
			language:    language.Java,
		},
		"node": {
			commandline: []string{"node", filepath.Join(curDir, "testdata", "server.js")},
			language:    language.Node,
		},
	}

	serverDir := buildFakeServer(t)
	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(func() { cancel() })

			bin := filepath.Join(serverDir, test.commandline[0])
			cmd := exec.CommandContext(ctx, bin, test.commandline[1:]...)
			cmd.Env = append(cmd.Env, test.env...)
			err := cmd.Start()
			require.NoError(t, err)

			pid := cmd.Process.Pid

			proc, err := process.NewProcess(int32(pid))
			require.NoError(t, err, "could not create gopsutil process handle")

			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				resp := getCheckServices(collect, discovery.url)
				startEvent := findService(pid, resp.StartedServices)
				require.NotNilf(collect, startEvent, "could not find start event for pid %v", pid)

				referenceValue, err := proc.Percent(0)
				require.NoError(t, err, "could not get gopsutil cpu usage value")

				assert.Equal(collect, string(test.language), startEvent.Language)
				assert.Equal(collect, string(apm.Provided), startEvent.APMInstrumentation)
				assertStat(collect, *startEvent)
				assert.InDelta(collect, referenceValue, startEvent.CPUCores*100, 10)
			}, 30*time.Second, 100*time.Millisecond)
		})
	}
}

func assertStat(t assert.TestingT, svc model.Service) {
	proc, err := process.NewProcess(int32(svc.PID))
	if !assert.NoError(t, err) {
		return
	}

	meminfo, err := proc.MemoryInfo()
	if !assert.NoError(t, err) {
		return
	}

	// Allow a 20% variation to avoid potential flakiness due to difference in
	// time of sampling the RSS.
	assert.InEpsilon(t, meminfo.RSS, svc.RSS, 0.20)

	createTimeMs, err := proc.CreateTime()
	if !assert.NoError(t, err) {
		return
	}

	// The value returned by proc.CreateTime() can vary between invocations
	// since the BootTime (used internally in proc.CreateTime()) can vary when
	// the version of BootTimeWithContext which uses /proc/uptime is active in
	// gopsutil (either on Docker, or even outside of it due to a bug fixed in
	// v4.24.8:
	// https://github.com/shirou/gopsutil/commit/aa0b73dc6d5669de5bc9483c0655b1f9446317a9).
	//
	// This is due to an inherent race since the code in BootTimeWithContext
	// subtracts the uptime of the host from the current time, and there can be
	// in theory an unbounded amount of time between the read of /proc/uptime
	// and the retrieval of the current time. Allow a 10 second diff as a
	// reasonable value.
	assert.InDelta(t, uint64(createTimeMs), svc.StartTimeMilli, 10000)
}

func TestCommandLineSanitization(t *testing.T) {
	serverDir := buildFakeServer(t)
	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	bin := filepath.Join(serverDir, "node")

	actualCommandLine := []string{bin, "--password", "secret", strings.Repeat("A", maxCommandLine*10)}
	sanitizedCommandLine := []string{bin, "--password", "********", "placeholder"}
	sanitizedCommandLine[3] = strings.Repeat("A", maxCommandLine-(len(bin)+len(sanitizedCommandLine[1])+len(sanitizedCommandLine[2])))

	cmd := exec.CommandContext(ctx, bin, actualCommandLine[1:]...)
	require.NoError(t, cmd.Start())

	pid := cmd.Process.Pid

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getCheckServices(collect, discovery.url)
		startEvent := findService(pid, resp.StartedServices)
		require.NotNilf(collect, startEvent, "could not find start event for pid %v", pid)
		assert.Equal(collect, sanitizedCommandLine, startEvent.CommandLine)
	}, 30*time.Second, 100*time.Millisecond)
}

func TestNodeDocker(t *testing.T) {
	cert, key, err := testutil.GetCertsPaths()
	require.NoError(t, err)

	require.NoError(t, nodejs.RunServerNodeJS(t, key, cert, "4444"))
	nodeJSPID, err := nodejs.GetNodeJSDockerPID()
	require.NoError(t, err)

	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

	pid := int(nodeJSPID)

	proc, err := process.NewProcess(int32(pid))
	require.NoError(t, err, "could not create gopsutil process handle")

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getCheckServices(collect, discovery.url)
		startEvent := findService(pid, resp.StartedServices)
		require.NotNilf(collect, startEvent, "could not find start event for pid %v", pid)

		referenceValue, err := proc.Percent(0)
		require.NoError(collect, err, "could not get gopsutil cpu usage value")

		// test@... changed to test_... due to normalization.
		assert.Equal(collect, "test_nodejs-https-server", startEvent.GeneratedName)
		assert.Equal(collect, string(usm.Nodejs), startEvent.GeneratedNameSource)
		assert.Equal(collect, "provided", startEvent.APMInstrumentation)
		assert.Equal(collect, "web_service", startEvent.Type)
		assertStat(collect, *startEvent)
		assert.InDelta(collect, referenceValue, startEvent.CPUCores*100, 10)
	}, 30*time.Second, 100*time.Millisecond)
}

func TestAPMInstrumentationProvidedWithMaps(t *testing.T) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	for _, test := range []struct {
		alias    string
		lib      string
		language language.Language
	}{
		{
			alias: "python",
			// We need the process to map something in a directory called
			// "site-packages/ddtrace". The actual mapped file does not matter.
			lib: filepath.Join(curDir,
				"..", "..", "..", "..",
				"network", "usm", "testdata",
				"site-packages", "ddtrace",
				fmt.Sprintf("libssl.so.%s", runtime.GOARCH)),
			language: language.Python,
		},
		{
			alias:    "dotnet",
			lib:      filepath.Join(curDir, "testdata", "Datadog.Trace.dll"),
			language: language.DotNet,
		},
	} {
		t.Run(test.alias, func(t *testing.T) {
			fmapper := fileopener.BuildFmapper(t)
			fake := makeAlias(t, test.alias, fmapper)

			// Give the process a listening socket
			listener, err := net.Listen("tcp", "")
			require.NoError(t, err)
			f, err := listener.(*net.TCPListener).File()
			listener.Close()
			require.NoError(t, err)
			t.Cleanup(func() { f.Close() })
			disableCloseOnExec(t, f)

			cmd, err := fileopener.OpenFromProcess(t, fake, test.lib)
			require.NoError(t, err)

			discovery := setupDiscoveryModule(t)
			discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

			pid := cmd.Process.Pid
			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				resp := getCheckServices(collect, discovery.url)

				// Start event assert
				startEvent := findService(pid, resp.StartedServices)
				require.NotNilf(collect, startEvent, "could not find start event for pid %v", pid)
				assert.Equal(collect, string(test.language), startEvent.Language)
				assert.Equal(collect, string(apm.Provided), startEvent.APMInstrumentation)
				assertStat(collect, *startEvent)
			}, 30*time.Second, 100*time.Millisecond)
		})
	}
}

// Check that we can get listening processes in other namespaces.
func TestNamespaces(t *testing.T) {
	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

	// Needed when changing namespaces
	runtime.LockOSThread()
	t.Cleanup(func() { runtime.UnlockOSThread() })

	origNs, err := netns.Get()
	require.NoError(t, err)
	t.Cleanup(func() { netns.Set(origNs) })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	t.Cleanup(func() { cancel() })

	expectedPorts := make(map[int]int)

	var pids []int
	for i := 0; i < 3; i++ {
		ns, err := netns.New()
		require.NoError(t, err)
		t.Cleanup(func() { ns.Close() })

		listener, err := net.Listen("tcp", "")
		require.NoError(t, err)
		port := listener.Addr().(*net.TCPAddr).Port
		f, err := listener.(*net.TCPListener).File()
		listener.Close()

		// Disable close-on-exec so that the sleep gets it
		require.NoError(t, err)
		t.Cleanup(func() { f.Close() })
		disableCloseOnExec(t, f)

		cmd := exec.CommandContext(ctx, "sleep", "1000")
		err = cmd.Start()
		require.NoError(t, err)
		f.Close()
		ns.Close()

		pids = append(pids, cmd.Process.Pid)
		expectedPorts[cmd.Process.Pid] = port
	}

	netns.Set(origNs)

	seen := make(map[int]model.Service)
	// Eventually to give the processes time to start
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getCheckServices(collect, discovery.url)
		for _, s := range resp.StartedServices {
			seen[s.PID] = s
		}

		for _, pid := range pids {
			require.Contains(collect, seen, pid)
			assert.Contains(collect, seen[pid].Ports, uint16(expectedPorts[pid]))
		}
	}, 30*time.Second, 100*time.Millisecond)
}

// Check that we are able to find services inside Docker containers.
func TestDocker(t *testing.T) {
	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

	dir, _ := testutil.CurDir()
	scanner, err := globalutils.NewScanner(regexp.MustCompile("Serving.*"), globalutils.NoPattern)
	require.NoError(t, err, "failed to create pattern scanner")

	dockerCfg := dockerutils.NewComposeConfig(
		dockerutils.NewBaseConfig(
			"foo-server",
			scanner,
		),
		filepath.Join(dir, "testdata", "docker-compose.yml"))
	err = dockerutils.Run(t, dockerCfg)
	require.NoError(t, err)

	proc, err := procfs.NewDefaultFS()
	require.NoError(t, err)
	processes, err := proc.AllProcs()
	require.NoError(t, err)
	pid1111 := 0
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		for _, process := range processes {
			comm, err := process.Comm()
			if err != nil {
				continue
			}
			if comm == "python-1111" {
				pid1111 = process.PID
				discovery.setProcessContainer(
					pid1111,
					"dummyCID",
					[]string{ // Collector tags from container
						"sometag:somevalue",
					},
					[]string{ // Tags from tagger
						"kube_service:kube_foo", // Should not have priority compared to app tag, for service naming
						"app:foo_from_app_tag",
					},
				)
				break
			}
		}
		assert.NotZero(collect, pid1111)
	}, time.Second*10, time.Millisecond*20)

	// First endpoint call will not contain any events, because the service is
	// still consider a potential service. The second call will have the events.
	_ = getCheckServices(t, discovery.url)
	resp := getCheckServices(t, discovery.url)

	// Assert events
	startEvent := findService(pid1111, resp.StartedServices)
	require.NotNilf(t, startEvent, "could not find start event for pid %v", pid1111)
	require.Contains(t, startEvent.Ports, uint16(1234))
	require.Contains(t, startEvent.ContainerID, "dummyCID")
	require.Contains(t, startEvent.GeneratedName, "http.server")
	require.Contains(t, startEvent.GeneratedNameSource, string(usm.CommandLine))
	require.Contains(t, startEvent.ContainerServiceName, "foo_from_app_tag")
	require.Contains(t, startEvent.ContainerServiceNameSource, "app")
	require.ElementsMatch(t, startEvent.ContainerTags, []string{
		"sometag:somevalue",
		"kube_service:kube_foo",
		"app:foo_from_app_tag",
	})
	require.Contains(t, startEvent.Type, "web_service")
	require.Equal(t, startEvent.LastHeartbeat, mockedTime.Unix())
}

func newDiscoveryNetwork(t testing.TB, tp core.TimeProvider, getNetworkCollector networkCollectorFactory) *discovery {
	mockWmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		compcore.MockBundle(),
		fx.Supply(context.Background()),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))
	mockTagger := taggerfxmock.SetupFakeTagger(t)

	return newDiscoveryWithNetwork(mockWmeta, mockTagger, tp, getNetworkCollector)
}

func newDiscovery(t testing.TB, tp core.TimeProvider) *discovery {
	return newDiscoveryNetwork(t, tp, func(_ *core.DiscoveryConfig) (core.NetworkCollector, error) {
		return nil, nil
	})
}

// Check that the cache is cleaned when procceses die.
func TestCache(t *testing.T) {
	var err error

	discovery := newDiscoveryNetwork(t, core.RealTime{}, newNetworkCollector)
	// Reduce update time to make sure we exercise network stats code paths.
	discovery.core.Config.NetworkStatsPeriod = 1 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	f, _ := startTCPServer(t, "tcp4", "")
	defer f.Close()

	disableCloseOnExec(t, f)

	var serviceNames []string
	var cmds []*exec.Cmd

	for i := 0; i < 10; i++ {
		cmd := exec.CommandContext(ctx, "sleep", "100")
		name := fmt.Sprintf("foo%d", i)
		env := fmt.Sprintf("DD_SERVICE=%s", name)
		cmd.Env = append(cmd.Env, env)
		err = cmd.Start()
		require.NoError(t, err)

		cmds = append(cmds, cmd)
		serviceNames = append(serviceNames, name)
	}
	f.Close()

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		_, err = discovery.getCheckServices(core.DefaultParams())
		require.NoError(collect, err)

		for _, cmd := range cmds {
			pid := int32(cmd.Process.Pid)
			assert.Contains(collect, discovery.core.Cache, pid)
		}
	}, 10*time.Second, 100*time.Millisecond)

	for i, cmd := range cmds {
		pid := int32(cmd.Process.Pid)
		require.Equal(t, serviceNames[i], discovery.core.Cache[pid].DDService)
		require.False(t, discovery.core.Cache[pid].DDServiceInjected)
	}

	cancel()
	for _, cmd := range cmds {
		cmd.Wait()
	}

	_, err = discovery.getCheckServices(core.DefaultParams())
	require.NoError(t, err)

	for _, cmd := range cmds {
		pid := cmd.Process.Pid
		require.NotContains(t, discovery.core.Cache, int32(pid))
	}

	// Add some PIDs to noPortTries to verify it gets cleaned up
	discovery.noPortTries[int32(1)] = 0

	discovery.Close()
	require.Empty(t, discovery.core.Cache)
	require.Empty(t, discovery.noPortTries)
	require.Empty(t, discovery.core.RunningServices)

	// Calling getCheckServices after Close is weird but it can happen in practice
	// due to the way system-probe shuts down, so make sure it doesn't panic.
	_, err = discovery.getCheckServices(core.DefaultParams())
	require.NoError(t, err)
	_, err = discovery.getCheckServices(core.DefaultParams())
	require.NoError(t, err)
}

func TestMaxPortCheck(t *testing.T) {
	// Start a process that will be ignored due to no ports
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, "sleep", "100")
	err := cmd.Start()
	require.NoError(t, err)
	pid := int32(cmd.Process.Pid)

	serverf, _ := startTCPServer(t, "tcp4", "")
	t.Cleanup(func() { serverf.Close() })

	selfPid := os.Getpid()

	params := core.DefaultParams()
	params.HeartbeatTime = 0

	mockCtrl := gomock.NewController(t)
	mTimeProvider := NewMocktimeProvider(mockCtrl)
	mTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()
	discovery := newDiscovery(t, mTimeProvider)

	for i := 0; i < maxPortCheckTries-5; i++ {
		_, err = discovery.getCheckServices(params)
		require.NoError(t, err)
	}

	discovery.mux.RLock()
	require.Contains(t, discovery.noPortTries, pid, "process should be in noPortTries")
	require.NotContains(t, discovery.noPortTries, selfPid, "self should not be in noPortTries")
	discovery.mux.RUnlock()

	for i := 0; i < 5; i++ {
		_, err = discovery.getCheckServices(params)
		require.NoError(t, err)
	}

	discovery.mux.RLock()
	require.NotContains(t, discovery.noPortTries, pid, "process should be removed from noPortTries")
	require.Contains(t, discovery.core.IgnorePids, pid, "process should be in ignorePids")
	discovery.mux.RUnlock()

	err = cmd.Process.Kill()
	require.NoError(t, err)
	err = cmd.Wait()
	require.Error(t, err)

	// Call getServices to trigger cleanup
	_, err = discovery.getCheckServices(params)
	require.NoError(t, err)

	discovery.mux.RLock()
	require.NotContains(t, discovery.noPortTries, pid, "process should be removed from noPortTries")
	require.NotContains(t, discovery.core.IgnorePids, pid, "process should not be in ignorePids")
	discovery.mux.RUnlock()
}

// addSockets adds only listening sockets to a map to be used for later looksups.
func addSockets[P procfs.NetTCP | procfs.NetUDP](sockMap map[uint64]socketInfo, sockets P,
	family network.ConnectionFamily, ctype network.ConnectionType, state uint64,
) {
	for _, sock := range sockets {
		if sock.St != state {
			continue
		}
		port := uint16(sock.LocalPort)
		if state == udpListen && network.IsPortInEphemeralRange(family, ctype, port) == network.EphemeralTrue {
			continue
		}
		sockMap[sock.Inode] = socketInfo{port: port}
	}
}

func getNsInfoOld(pid int) (*namespaceInfo, error) {
	path := kernel.HostProc(fmt.Sprintf("%d", pid))
	proc, err := procfs.NewFS(path)
	if err != nil {
		return nil, err
	}

	TCP, _ := proc.NetTCP()
	UDP, _ := proc.NetUDP()
	TCP6, _ := proc.NetTCP6()
	UDP6, _ := proc.NetUDP6()

	listeningSockets := make(map[uint64]socketInfo)

	addSockets(listeningSockets, TCP, network.AFINET, network.TCP, tcpListen)
	addSockets(listeningSockets, TCP6, network.AFINET6, network.TCP, tcpListen)
	addSockets(listeningSockets, UDP, network.AFINET, network.UDP, udpListen)
	addSockets(listeningSockets, UDP6, network.AFINET6, network.UDP, udpListen)

	return &namespaceInfo{
		listeningSockets: listeningSockets,
	}, nil
}

func TestGetNSInfo(t *testing.T) {
	lTCP, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	defer lTCP.Close()

	res, err := getNsInfo(os.Getpid())
	require.NoError(t, err)
	resOld, err := getNsInfoOld(os.Getpid())
	require.NoError(t, err)
	require.Equal(t, res, resOld)
}

func BenchmarkGetNSInfo(b *testing.B) {
	sockets := make([]net.Listener, 0)
	for i := 0; i < 100; i++ {
		l, err := net.Listen("tcp", "localhost:0")
		require.NoError(b, err)
		sockets = append(sockets, l)
	}
	defer func() {
		for _, l := range sockets {
			l.Close()
		}
	}()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		getNsInfo(os.Getpid())
	}
}

func BenchmarkGetNSInfoOld(b *testing.B) {
	sockets := make([]net.Listener, 0)
	for i := 0; i < 100; i++ {
		l, err := net.Listen("tcp", "localhost:0")
		require.NoError(b, err)
		sockets = append(sockets, l)
	}
	defer func() {
		for _, l := range sockets {
			l.Close()
		}
	}()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		getNsInfoOld(os.Getpid())
	}
}

func makeHTTPRequest(t *testing.T, method, url string) *http.Response {
	req, err := http.NewRequest(method, url, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { resp.Body.Close() })

	require.Equal(t, http.StatusOK, resp.StatusCode)

	return resp
}

func getStateResponse(t *testing.T, url string) *state {
	stateURL := url + "/" + string(config.DiscoveryModule) + "/state"
	resp := makeHTTPRequest(t, http.MethodGet, stateURL)

	var state state
	err := json.NewDecoder(resp.Body).Decode(&state)
	require.NoError(t, err)

	resp.Body.Close()

	return &state
}

func createTracerMemfd(t *testing.T, data []byte) int {
	t.Helper()
	fd, err := unix.MemfdCreate("datadog-tracer-info-xxx", 0)
	require.NoError(t, err)
	t.Cleanup(func() { unix.Close(fd) })
	err = unix.Ftruncate(fd, int64(len(data)))
	require.NoError(t, err)
	mappedData, err := unix.Mmap(fd, 0, len(data), unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED)
	require.NoError(t, err)
	copy(mappedData, data)
	err = unix.Munmap(mappedData)
	require.NoError(t, err)
	return fd
}

func TestValidInvalidTracerMetadata(t *testing.T) {
	discovery := newDiscovery(t, nil)
	require.NotEmpty(t, discovery)
	self := os.Getpid()

	t.Run("valid metadata", func(t *testing.T) {
		// Test with valid metadata from file
		curDir, err := testutil.CurDir()
		require.NoError(t, err)
		testDataPath := filepath.Join(curDir, "testdata/tracer_cpp.data")
		data, err := os.ReadFile(testDataPath)
		require.NoError(t, err)

		createTracerMemfd(t, data)

		info, err := discovery.getServiceInfo(int32(self))
		require.NoError(t, err)
		require.Equal(t, language.CPlusPlus, language.Language(info.Language))
		require.Equal(t, apm.Provided, apm.Instrumentation(info.APMInstrumentation))
	})

	t.Run("invalid metadata", func(t *testing.T) {
		createTracerMemfd(t, []byte("invalid data"))

		info, err := discovery.getServiceInfo(int32(self))
		require.NoError(t, err)
		require.Equal(t, apm.None, apm.Instrumentation(info.APMInstrumentation))
	})
}

func TestStateEndpoint(t *testing.T) {
	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

	serverf, _ := startTCPServer(t, "tcp4", "")
	t.Cleanup(func() { serverf.Close() })
	pid := os.Getpid()

	_ = getCheckServices(t, discovery.url)
	resp := getCheckServices(t, discovery.url)
	startEvent := findService(pid, resp.StartedServices)
	require.NotNilf(t, startEvent, "could not find start event for pid %v", pid)

	state := getStateResponse(t, discovery.url)
	require.NotEmpty(t, state.Cache)

	var serviceInfo *model.Service
	for _, service := range state.Cache {
		if service.PID == pid {
			serviceInfo = service
			break
		}
	}
	require.NotNil(t, serviceInfo, "could not find service with pid %v in cache", pid)

	require.Equal(t, pid, serviceInfo.PID)
	require.Equal(t, mockedTime.Unix(), serviceInfo.LastHeartbeat)

	require.NotNil(t, state.NoPortTries)
	require.NotNil(t, state.PotentialServices)
	require.NotNil(t, state.RunningServices)
	require.NotNil(t, state.IgnorePids)
}

func TestNetworkStatsEndpoint(t *testing.T) {
	tests := []struct {
		name           string
		pids           string
		networkEnabled bool
		expectedCode   int
		expectedBody   *model.NetworkStatsResponse
	}{
		{
			name:           "network stats disabled",
			pids:           "123",
			networkEnabled: false,
			expectedCode:   http.StatusServiceUnavailable,
		},
		{
			name:           "missing pids parameter",
			pids:           "",
			networkEnabled: true,
			expectedCode:   http.StatusBadRequest,
		},
		{
			name:           "invalid pid format",
			pids:           "abc",
			networkEnabled: true,
			expectedCode:   http.StatusBadRequest,
		},
		{
			name:           "valid pids",
			pids:           "123,456",
			networkEnabled: true,
			expectedCode:   http.StatusOK,
			expectedBody: &model.NetworkStatsResponse{
				Stats: map[int]model.NetworkStats{
					123: {
						RxBytes: 1000,
						TxBytes: 2000,
					},
					456: {
						RxBytes: 3000,
						TxBytes: 4000,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock network collector
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()

			mockNetwork := core.NewMockNetworkCollector(mockCtrl)
			if tt.networkEnabled {
				// Only expect getStats to be called for valid pids
				if tt.expectedCode == http.StatusOK {
					mockNetwork.EXPECT().
						GetStats(core.PidSet{123: {}, 456: {}}).
						Return(map[uint32]core.NetworkStats{
							123: {Rx: 1000, Tx: 2000},
							456: {Rx: 3000, Tx: 4000},
						}, nil)
				}
				mockNetwork.EXPECT().Close().AnyTimes()
			}

			// Setup discovery module with mock network collector
			module := setupDiscoveryModuleWithNetwork(t, func(_ *core.DiscoveryConfig) (core.NetworkCollector, error) {
				if tt.networkEnabled {
					return mockNetwork, nil
				}
				return nil, fmt.Errorf("network stats collection is not enabled")
			})

			// Make request to network stats endpoint
			url := fmt.Sprintf("%s/%s/network-stats?pids=%s", module.url, config.DiscoveryModule, tt.pids)
			resp, err := http.Get(url)
			require.NoError(t, err)
			defer resp.Body.Close()

			// Check response
			assert.Equal(t, tt.expectedCode, resp.StatusCode)

			if tt.expectedCode == http.StatusOK {
				var body model.NetworkStatsResponse
				err := json.NewDecoder(resp.Body).Decode(&body)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedBody, &body)
			}
		})
	}
}
