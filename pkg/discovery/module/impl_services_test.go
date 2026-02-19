// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// This doesn't need BPF, but it's built with this tag to only run with
// system-probe tests.
//go:build test && linux_bpf && !cgo

package module

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/language"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/discovery/usm"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/tls/nodejs"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	globalutils "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

// getServices call the /discovery/services endpoint. It will perform a /proc scan
// to get the list of running pids and use them as the pids query param.
func getServices(t require.TestingT, url string) *model.ServicesResponse {
	location := url + "/" + string(config.DiscoveryModule) + pathServices
	params := &core.Params{
		NewPids: getRunningPids(t),
	}

	return makeRequest[model.ServicesResponse](t, location, params)
}

// Check that we get (only) listening processes for all expected protocols using the services endpoint.
func TestServicesBasic(t *testing.T) {
	discovery := setupDiscoveryModule(t)

	var expectedPIDs []int
	var unexpectedPIDs []int
	exceptedTCPPorts := make(map[int]int)
	exceptedUDPPorts := make(map[int]int)

	startTCP := func(proto string) {
		f, server := startTCPServer(t, proto, "")
		cmd := startProcessWithFile(t, f)
		expectedPIDs = append(expectedPIDs, cmd.Process.Pid)
		exceptedTCPPorts[cmd.Process.Pid] = server.Port

		f, _ = startTCPClient(t, proto, server)
		cmd = startProcessWithFile(t, f)
		unexpectedPIDs = append(unexpectedPIDs, cmd.Process.Pid)
	}

	startUDP := func(proto string) {
		f, server := startUDPServer(t, proto, ":8083")
		cmd := startProcessWithFile(t, f)
		expectedPIDs = append(expectedPIDs, cmd.Process.Pid)
		exceptedUDPPorts[cmd.Process.Pid] = server.Port

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
		resp := getServices(collect, discovery.url)
		for _, s := range resp.Services {
			seen[s.PID] = s
		}

		for _, pid := range expectedPIDs {
			require.Contains(collect, seen, pid)
			assert.Equal(collect, seen[pid].PID, pid)
			if expectedTCPPort, ok := exceptedTCPPorts[pid]; ok {
				require.Contains(collect, seen[pid].TCPPorts, uint16(expectedTCPPort))
			}
			if expectedUDPPort, ok := exceptedUDPPorts[pid]; ok {
				require.Contains(collect, seen[pid].UDPPorts, uint16(expectedUDPPort))
			}
		}
		for _, pid := range unexpectedPIDs {
			assert.NotContains(collect, seen, pid)
		}
	}, 30*time.Second, 100*time.Millisecond)
}

// Check that we get all listening ports for a process using the services endpoint
func TestServicesPorts(t *testing.T) {
	discovery := setupDiscoveryModule(t)

	var expectedTCPPorts []uint16
	var expectedUDPPorts []uint16
	var unexpectedTCPPorts []uint16
	var unexpectedUDPPorts []uint16

	startTCP := func(proto string) {
		serverf, server := startTCPServer(t, proto, "")
		t.Cleanup(func() { serverf.Close() })
		clientf, client := startTCPClient(t, proto, server)
		t.Cleanup(func() { clientf.Close() })

		expectedTCPPorts = append(expectedTCPPorts, uint16(server.Port))
		unexpectedTCPPorts = append(unexpectedTCPPorts, uint16(client.Port))
	}

	startUDP := func(proto string) {
		serverf, server := startUDPServer(t, proto, ":8083")
		t.Cleanup(func() { _ = serverf.Close() })
		clientf, client := startUDPClient(t, proto, server)
		t.Cleanup(func() { clientf.Close() })

		expectedUDPPorts = append(expectedUDPPorts, uint16(server.Port))
		unexpectedUDPPorts = append(unexpectedUDPPorts, uint16(client.Port))

		ephemeralf, ephemeral := startUDPServer(t, proto, "")
		t.Cleanup(func() { _ = ephemeralf.Close() })
		unexpectedUDPPorts = append(unexpectedUDPPorts, uint16(ephemeral.Port))
	}

	startTCP("tcp4")
	startTCP("tcp6")
	startUDP("udp4")
	startUDP("udp6")

	expectedTCPPortsMap := make(map[uint16]struct{}, len(expectedTCPPorts))
	expectedUDPPortsMap := make(map[uint16]struct{}, len(expectedUDPPorts))

	pid := os.Getpid()
	resp := getServices(t, discovery.url)
	svc := findService(pid, resp.Services)
	require.NotNilf(t, svc, "could not find service for pid %v", pid)

	for _, port := range expectedTCPPorts {
		expectedTCPPortsMap[port] = struct{}{}
		assert.Contains(t, svc.TCPPorts, port)
	}
	for _, port := range expectedUDPPorts {
		expectedUDPPortsMap[port] = struct{}{}
		assert.Contains(t, svc.UDPPorts, port)
	}
	for _, port := range unexpectedTCPPorts {
		// An unexpected port number can also be expected since v4 and v6 are
		// in the same list. Just skip the extra check in that case since it
		// should be rare.
		if _, alsoExpected := expectedTCPPortsMap[port]; alsoExpected {
			continue
		}

		// Do not assert about this since this check can spuriously fail since
		// the test infrastructure opens a listening TCP socket on an ephimeral
		// port, and since we mix the different protocols we could find that on
		// the unexpected port list.
		if slices.Contains(svc.TCPPorts, port) {
			t.Logf("unexpected TCP port %v also found", port)
		}
	}
	for _, port := range unexpectedUDPPorts {
		if _, alsoExpected := expectedUDPPortsMap[port]; alsoExpected {
			continue
		}

		if slices.Contains(svc.UDPPorts, port) {
			t.Logf("unexpected UDP port %v also found", port)
		}
	}
}

func TestServicesPortsLimits(t *testing.T) {
	discovery := setupDiscoveryModule(t)

	var expectedPorts []int

	openPort := func(address string) {
		serverf, server := startTCPServer(t, "tcp4", address)
		t.Cleanup(func() { serverf.Close() })

		expectedPorts = append(expectedPorts, server.Port)
	}

	openPort("127.0.0.1:8081")

	for range maxNumberOfPorts {
		openPort("")
	}

	openPort("127.0.0.1:8082")

	slices.Sort(expectedPorts)

	pid := os.Getpid()

	resp := getServices(t, discovery.url)
	svc := findService(pid, resp.Services)
	require.NotNilf(t, svc, "could not find service for pid %v", pid)

	assert.Contains(t, svc.TCPPorts, uint16(8081))
	assert.Contains(t, svc.TCPPorts, uint16(8082))
	assert.Len(t, svc.TCPPorts, maxNumberOfPorts)
	for i := range maxNumberOfPorts - 2 {
		assert.Contains(t, svc.TCPPorts, uint16(expectedPorts[i]))
	}
}

func TestServicesServiceName(t *testing.T) {
	discovery := setupDiscoveryModule(t)

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
	cmd.Env = append(cmd.Env, "DD_ENV=my😀dd-env")
	cmd.Env = append(cmd.Env, "DD_VERSION=my😀dd-version")
	cmd.Env = append(cmd.Env, "YET_OTHER_ENV=test")
	err = cmd.Start()
	require.NoError(t, err)
	f.Close()

	pid := cmd.Process.Pid
	var svc *model.Service
	// Eventually to give the processes time to start
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getServices(collect, discovery.url)
		svc = findService(pid, resp.Services)
		require.NotNilf(collect, svc, "could not find service for pid %v", pid)

		assert.Equal(collect, "foo😀bar", svc.UST.Service)
		assert.Equal(collect, "my😀dd-env", svc.UST.Env)
		assert.Equal(collect, "my😀dd-version", svc.UST.Version)

		assert.Equal(collect, "sleep", svc.GeneratedName)
		assert.Equal(collect, string(usm.CommandLine), svc.GeneratedNameSource)
	}, 30*time.Second, 100*time.Millisecond)

	// Verify tracer metadata
	assert.Equal(t, []tracermetadata.TracerMetadata{trMeta}, svc.TracerMetadata)
	assert.Equal(t, string(language.Go), svc.Language)
}

// TestServicesTracerMetadataWithoutPorts checks that processes with tracer metadata
// are discovered even when they have no open listening ports.
func TestServicesTracerMetadataWithoutPorts(t *testing.T) {
	discovery := setupDiscoveryModule(t)

	trMeta := tracermetadata.TracerMetadata{
		SchemaVersion:  1,
		RuntimeID:      "test-runtime-id-noports",
		TracerLanguage: "python",
		ServiceName:    "background-worker",
	}
	data, err := trMeta.MarshalMsg(nil)
	require.NoError(t, err)

	createTracerMemfd(t, data)

	// Create a process WITHOUT any listening ports - just a simple sleep
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	cmd := exec.CommandContext(ctx, "sleep", "1000")
	cmd.Dir = "/tmp/"
	cmd.Env = append(cmd.Env, "DD_SERVICE=background-worker")
	cmd.Env = append(cmd.Env, "DD_ENV=test-env")
	cmd.Env = append(cmd.Env, "DD_VERSION=1.0.0")
	err = cmd.Start()
	require.NoError(t, err)

	pid := cmd.Process.Pid
	var svc *model.Service

	// Eventually to give the processes time to start
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getServices(collect, discovery.url)
		svc = findService(pid, resp.Services)
		require.NotNilf(collect, svc, "could not find service for pid %v", pid)

		// Verify the service was discovered even without ports
		assert.Empty(collect, svc.TCPPorts, "should have no TCP ports")
		assert.Empty(collect, svc.UDPPorts, "should have no UDP ports")

		// Verify UST fields from environment variables
		assert.Equal(collect, "background-worker", svc.UST.Service)
		assert.Equal(collect, "test-env", svc.UST.Env)
		assert.Equal(collect, "1.0.0", svc.UST.Version)

		// Verify basic service info
		assert.Equal(collect, "sleep", svc.GeneratedName)
		assert.Equal(collect, string(usm.CommandLine), svc.GeneratedNameSource)
	}, 30*time.Second, 100*time.Millisecond)

	// Verify tracer metadata
	assert.Equal(t, []tracermetadata.TracerMetadata{trMeta}, svc.TracerMetadata)
	assert.Equal(t, string(language.Python), svc.Language)
}

// TestServicesLogsWithoutPorts checks that processes with open log files
// are discovered even when they have no listening ports or tracer metadata.
func TestServicesLogsWithoutPorts(t *testing.T) {
	discovery := setupDiscoveryModule(t)

	// Create a temporary log file path
	logFile, err := os.CreateTemp("/tmp", "test-service-*.log")
	require.NoError(t, err)
	logFilePath := logFile.Name()
	logFile.Close()
	t.Cleanup(func() {
		os.Remove(logFilePath)
	})

	// Open the file with O_WRONLY | O_APPEND flags (required for log file detection)
	logFd, err := os.OpenFile(logFilePath, os.O_WRONLY|os.O_APPEND, 0644)
	require.NoError(t, err)

	// Write something to the log to make it more realistic
	_, err = logFd.WriteString("Service started\n")
	require.NoError(t, err)

	// Disable close-on-exec so the sleep process inherits the log file
	disableCloseOnExec(t, logFd)

	// Create a process WITHOUT any listening ports or tracer metadata
	// but WITH an open log file
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	cmd := exec.CommandContext(ctx, "sleep", "1000")
	cmd.Dir = "/tmp/"
	err = cmd.Start()
	require.NoError(t, err)

	// Close the log file in the parent process (the test)
	// The child process (sleep) still has it open
	logFd.Close()

	pid := cmd.Process.Pid
	var svc *model.Service

	// Eventually to give the processes time to start
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getServices(collect, discovery.url)
		svc = findService(pid, resp.Services)
		require.NotNilf(collect, svc, "could not find service for pid %v", pid)

		// Verify the service was discovered even without ports or tracer metadata
		assert.Empty(collect, svc.TCPPorts, "should have no TCP ports")
		assert.Empty(collect, svc.UDPPorts, "should have no UDP ports")
		assert.Empty(collect, svc.TracerMetadata, "should have no tracer metadata")

		// Verify the log file is present
		assert.NotEmpty(collect, svc.LogFiles, "should have log files")

		// Verify basic service info
		assert.Equal(collect, "sleep", svc.GeneratedName)
		assert.Equal(collect, string(usm.CommandLine), svc.GeneratedNameSource)
	}, 30*time.Second, 100*time.Millisecond)

	// Verify at least one log file path contains our temp file name
	found := false
	logFileName := filepath.Base(logFilePath)
	for _, lf := range svc.LogFiles {
		if strings.Contains(lf, logFileName) {
			found = true
			break
		}
	}
	assert.True(t, found, "expected to find log file %s in LogFiles: %v", logFileName, svc.LogFiles)
}

func TestServicesAPMInstrumentationProvided(t *testing.T) {
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
	}

	serverDir := buildFakeServer(t)
	discovery := setupDiscoveryModule(t)

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

			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				resp := getServices(collect, discovery.url)
				startEvent := findService(pid, resp.Services)
				require.NotNilf(collect, startEvent, "could not find start event for pid %v", pid)

				assert.Equal(collect, startEvent.PID, pid)
				assert.Equal(collect, string(test.language), startEvent.Language)
				assert.Equal(collect, true, startEvent.APMInstrumentation)
			}, 30*time.Second, 100*time.Millisecond)
		})
	}
}

func TestServicesNodeDocker(t *testing.T) {
	cert, key, err := testutil.GetCertsPaths()
	require.NoError(t, err)

	require.NoError(t, nodejs.RunServerNodeJS(t, key, cert, "4444"))
	nodeJSPID, err := nodejs.GetNodeJSDockerPID()
	require.NoError(t, err)

	discovery := setupDiscoveryModule(t)

	pid := int(nodeJSPID)

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getServices(collect, discovery.url)
		svc := findService(pid, resp.Services)
		require.NotNilf(collect, svc, "could not find start event for pid %v", pid)

		assert.Equal(collect, svc.PID, pid)
		assert.Equal(collect, "test@nodejs-https-server", svc.GeneratedName)
		assert.Equal(collect, string(usm.Nodejs), svc.GeneratedNameSource)
		assert.Equal(collect, false, svc.APMInstrumentation)
		assert.Equal(collect, "web_service", svc.Type)
	}, 30*time.Second, 100*time.Millisecond)
}

func TestServicesAPMInstrumentationProvidedWithMaps(t *testing.T) {
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
				"..", "..",
				"network", "usm", "testdata",
				"site-packages", "ddtrace",
				"libssl.so."+runtime.GOARCH),
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

			pid := cmd.Process.Pid
			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				resp := getServices(collect, discovery.url)

				// Service assert
				svc := findService(pid, resp.Services)
				require.NotNilf(collect, svc, "could not find start event for pid %v", pid)
				assert.Equal(collect, svc.PID, pid)
				assert.Equal(collect, string(test.language), svc.Language)
				assert.Equal(collect, true, svc.APMInstrumentation)
			}, 30*time.Second, 100*time.Millisecond)
		})
	}
}

// Check that we can get listening processes in other namespaces using the services endpoint.
func TestServicesNamespaces(t *testing.T) {
	discovery := setupDiscoveryModule(t)

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
	for range 3 {
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
		resp := getServices(collect, discovery.url)
		for _, s := range resp.Services {
			seen[s.PID] = s
		}

		for _, pid := range pids {
			require.Contains(collect, seen, pid)
			assert.Contains(collect, seen[pid].TCPPorts, uint16(expectedPorts[pid]))
		}
	}, 30*time.Second, 100*time.Millisecond)
}

// Check that we are able to find services inside Docker containers using the services endpoint.
func TestServicesDocker(t *testing.T) {
	discovery := setupDiscoveryModule(t)

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
				break
			}
		}
		assert.NotZero(collect, pid1111)
	}, time.Second*10, time.Millisecond*20)

	resp := getServices(t, discovery.url)

	// Assert events
	svc := findService(pid1111, resp.Services)
	require.NotNilf(t, svc, "could not find start event for pid %v", pid1111)
	require.Contains(t, svc.TCPPorts, uint16(1234))
	require.Contains(t, svc.GeneratedName, "http.server")
	require.Contains(t, svc.GeneratedNameSource, string(usm.CommandLine))
	require.Contains(t, svc.Type, "web_service")
}
