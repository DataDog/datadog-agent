// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// This doesn't need BPF, but it's built with this tag to only run with
// system-probe tests.
//go:build test && linux_bpf

package module

import (
	"context"
	"fmt"
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
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/core"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/usm"
	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/tls/nodejs"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	globalutils "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

// getServices call the /discovery/services endpoint. It will perform a /proc scan
// to get the list of running pids and use them as the pids query param.
func getServices(t require.TestingT, url string) *model.ServicesEndpointResponse {
	location := url + "/" + string(config.DiscoveryModule) + pathServices
	params := &core.Params{
		Pids: getRunningPids(t),
	}

	return makeRequest[model.ServicesEndpointResponse](t, location, params)
}

// Check that we get (only) listening processes for all expected protocols using the services endpoint.
func TestServicesBasic(t *testing.T) {
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
		resp := getServices(collect, discovery.url)
		for _, s := range resp.Services {
			seen[s.PID] = s
		}

		for _, pid := range expectedPIDs {
			require.Contains(collect, seen, pid)
			assert.Equal(collect, seen[pid].PID, pid)
			assert.Greater(collect, seen[pid].StartTimeMilli, uint64(0))
			require.Contains(collect, seen[pid].Ports, uint16(expectedPorts[pid]))
		}
		for _, pid := range unexpectedPIDs {
			assert.NotContains(collect, seen, pid)
		}
	}, 30*time.Second, 100*time.Millisecond)
}

// Check that we get all listening ports for a process using the services endpoint
func TestServicesPorts(t *testing.T) {
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

	expectedPortsMap := make(map[uint16]struct{}, len(expectedPorts))

	pid := os.Getpid()
	resp := getServices(t, discovery.url)
	svc := findService(pid, resp.Services)
	require.NotNilf(t, svc, "could not find service for pid %v", pid)

	for _, port := range expectedPorts {
		expectedPortsMap[port] = struct{}{}
		assert.Contains(t, svc.Ports, port)
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
		if slices.Contains(svc.Ports, port) {
			t.Logf("unexpected port %v also found", port)
		}
	}
}

func TestServicesPortsLimits(t *testing.T) {
	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

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

	assert.Contains(t, svc.Ports, uint16(8081))
	assert.Contains(t, svc.Ports, uint16(8082))
	assert.Len(t, svc.Ports, maxNumberOfPorts)
	for i := range maxNumberOfPorts - 2 {
		assert.Contains(t, svc.Ports, uint16(expectedPorts[i]))
	}
}

func TestServicesServiceName(t *testing.T) {
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
	var svc *model.Service
	// Eventually to give the processes time to start
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getServices(collect, discovery.url)
		svc = findService(pid, resp.Services)
		require.NotNilf(collect, svc, "could not find service for pid %v", pid)

		// Non-ASCII character removed due to normalization.
		assert.Equal(collect, "foo_bar", svc.DDService)
		assert.Equal(collect, "sleep", svc.GeneratedName)
		assert.Equal(collect, string(usm.CommandLine), svc.GeneratedNameSource)
		assert.False(collect, svc.DDServiceInjected)
	}, 30*time.Second, 100*time.Millisecond)

	// Verify tracer metadata
	assert.Equal(t, []tracermetadata.TracerMetadata{trMeta}, svc.TracerMetadata)
	assert.Equal(t, string(language.Go), svc.Language)
}

func testServicesCaptureWrappedCommands(t *testing.T, script string, commandWrapper []string, validator func(service model.Service) bool) {
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
		resp := getServices(collect, discovery.url)
		startEvent := findService(pid, resp.Services)
		require.NotNilf(collect, startEvent, "could not find service for pid %v", pid)
		assert.True(collect, validator(*startEvent))
	}, 30*time.Second, 100*time.Millisecond)
}

func TestServicesPythonFromBashScript(t *testing.T) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	pythonScriptPath := filepath.Join(curDir, "testdata", "script.py")

	t.Run("PythonFromBashScript", func(t *testing.T) {
		testServicesCaptureWrappedCommands(t, pythonScriptPath, []string{"sh", "-c"}, func(service model.Service) bool {
			return service.Language == string(language.Python)
		})
	})
	t.Run("DirectPythonScript", func(t *testing.T) {
		testServicesCaptureWrappedCommands(t, pythonScriptPath, nil, func(service model.Service) bool {
			return service.Language == string(language.Python)
		})
	})
}

func TestServicesAPMInstrumentationProvided(t *testing.T) {
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

			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				resp := getServices(collect, discovery.url)
				startEvent := findService(pid, resp.Services)
				require.NotNilf(collect, startEvent, "could not find start event for pid %v", pid)

				assert.Equal(collect, startEvent.PID, pid)
				assert.Greater(collect, startEvent.StartTimeMilli, uint64(0))
				assert.Equal(collect, string(test.language), startEvent.Language)
				assert.Equal(collect, string(apm.Provided), startEvent.APMInstrumentation)
			}, 30*time.Second, 100*time.Millisecond)
		})
	}
}

func TestServicesCommandLineSanitization(t *testing.T) {
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
		resp := getServices(collect, discovery.url)
		startEvent := findService(pid, resp.Services)
		require.NotNilf(collect, startEvent, "could not find start event for pid %v", pid)
		assert.Equal(collect, sanitizedCommandLine, startEvent.CommandLine)
	}, 30*time.Second, 100*time.Millisecond)
}

func TestServicesNodeDocker(t *testing.T) {
	cert, key, err := testutil.GetCertsPaths()
	require.NoError(t, err)

	require.NoError(t, nodejs.RunServerNodeJS(t, key, cert, "4444"))
	nodeJSPID, err := nodejs.GetNodeJSDockerPID()
	require.NoError(t, err)

	discovery := setupDiscoveryModule(t)
	discovery.mockTimeProvider.EXPECT().Now().Return(mockedTime).AnyTimes()

	pid := int(nodeJSPID)

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		resp := getServices(collect, discovery.url)
		svc := findService(pid, resp.Services)
		require.NotNilf(collect, svc, "could not find start event for pid %v", pid)

		// test@... changed to test_... due to normalization.
		assert.Equal(collect, svc.PID, pid)
		assert.Greater(collect, svc.StartTimeMilli, uint64(0))
		assert.Equal(collect, "test_nodejs-https-server", svc.GeneratedName)
		assert.Equal(collect, string(usm.Nodejs), svc.GeneratedNameSource)
		assert.Equal(collect, "provided", svc.APMInstrumentation)
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
				resp := getServices(collect, discovery.url)

				// Service assert
				svc := findService(pid, resp.Services)
				require.NotNilf(collect, svc, "could not find start event for pid %v", pid)
				assert.Equal(collect, svc.PID, pid)
				assert.Greater(collect, svc.StartTimeMilli, uint64(0))
				assert.Equal(collect, string(test.language), svc.Language)
				assert.Equal(collect, string(apm.Provided), svc.APMInstrumentation)
			}, 30*time.Second, 100*time.Millisecond)
		})
	}
}

// Check that we can get listening processes in other namespaces using the services endpoint.
func TestServicesNamespaces(t *testing.T) {
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
			assert.Contains(collect, seen[pid].Ports, uint16(expectedPorts[pid]))
		}
	}, 30*time.Second, 100*time.Millisecond)
}

// Check that we are able to find services inside Docker containers using the services endpoint.
func TestServicesDocker(t *testing.T) {
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
				break
			}
		}
		assert.NotZero(collect, pid1111)
	}, time.Second*10, time.Millisecond*20)

	resp := getServices(t, discovery.url)

	// Assert events
	svc := findService(pid1111, resp.Services)
	require.NotNilf(t, svc, "could not find start event for pid %v", pid1111)
	require.Contains(t, svc.Ports, uint16(1234))
	require.Contains(t, svc.GeneratedName, "http.server")
	require.Contains(t, svc.GeneratedNameSource, string(usm.CommandLine))
	require.Contains(t, svc.Type, "web_service")
}
