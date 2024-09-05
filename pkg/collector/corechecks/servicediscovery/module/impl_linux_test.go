// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// This doesn't need BPF but it's built with this tag to only run with
// system-probe tests.
//go:build linux_bpf

package module

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"net/http/httptest"

	gorillamux "github.com/gorilla/mux"
	"github.com/prometheus/procfs"
	"github.com/shirou/gopsutil/v3/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/comp/core"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	wmmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/tls/nodejs"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func setupDiscoveryModule(t *testing.T) string {
	t.Helper()

	wmeta := fxutil.Test[workloadmeta.Component](t,
		core.MockBundle(),
		wmmock.MockModule(workloadmeta.NewParams()),
	)
	mux := gorillamux.NewRouter()
	cfg := &types.Config{
		Enabled: true,
		EnabledModules: map[types.ModuleName]struct{}{
			config.DiscoveryModule: {},
		},
	}
	m := module.Factory{
		Name:             config.DiscoveryModule,
		ConfigNamespaces: []string{"discovery"},
		Fn:               NewDiscoveryModule,
		NeedsEBPF: func() bool {
			return false
		},
	}
	err := module.Register(cfg, mux, []module.Factory{m}, wmeta, nil)
	require.NoError(t, err)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func getServices(t *testing.T, url string) []model.Service {
	location := url + "/" + string(config.DiscoveryModule) + pathServices
	req, err := http.NewRequest(http.MethodGet, location, nil)
	require.NoError(t, err)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	res := &model.ServicesResponse{}
	err = json.NewDecoder(resp.Body).Decode(res)
	require.NoError(t, err)
	require.NotEmpty(t, res)

	return res.Services
}

func getServicesMap(t *testing.T, url string) map[int]model.Service {
	services := getServices(t, url)
	servicesMap := make(map[int]model.Service)
	for _, service := range services {
		servicesMap[service.PID] = service
	}

	return servicesMap
}

func startTCPServer(t *testing.T, proto string) (*os.File, *net.TCPAddr) {
	listener, err := net.Listen(proto, "")
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

func startUDPServer(t *testing.T, proto string) (*os.File, *net.UDPAddr) {
	lnPacket, err := net.ListenPacket(proto, "")
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
	url := setupDiscoveryModule(t)

	var expectedPIDs []int
	var unexpectedPIDs []int
	expectedPorts := make(map[int]int)

	var startTCP = func(proto string) {
		f, server := startTCPServer(t, proto)
		cmd := startProcessWithFile(t, f)
		expectedPIDs = append(expectedPIDs, cmd.Process.Pid)
		expectedPorts[cmd.Process.Pid] = server.Port

		f, _ = startTCPClient(t, proto, server)
		cmd = startProcessWithFile(t, f)
		unexpectedPIDs = append(unexpectedPIDs, cmd.Process.Pid)
	}

	var startUDP = func(proto string) {
		f, server := startUDPServer(t, proto)
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

	// Eventually to give the processes time to start
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		portMap := getServicesMap(t, url)
		for _, pid := range expectedPIDs {
			assert.Contains(collect, portMap, pid)
		}
		for _, pid := range unexpectedPIDs {
			assert.NotContains(collect, portMap, pid)
		}
	}, 30*time.Second, 100*time.Millisecond)

	serviceMap := getServicesMap(t, url)
	for _, pid := range expectedPIDs {
		require.Contains(t, serviceMap[pid].Ports, uint16(expectedPorts[pid]))
		assertStat(t, serviceMap[pid])
	}
}

// Check that we get all listening ports for a process
func TestPorts(t *testing.T) {
	url := setupDiscoveryModule(t)

	var expectedPorts []uint16
	var unexpectedPorts []uint16

	var startTCP = func(proto string) {
		serverf, server := startTCPServer(t, proto)
		t.Cleanup(func() { serverf.Close() })
		clientf, client := startTCPClient(t, proto, server)
		t.Cleanup(func() { clientf.Close() })

		expectedPorts = append(expectedPorts, uint16(server.Port))
		unexpectedPorts = append(unexpectedPorts, uint16(client.Port))
	}

	var startUDP = func(proto string) {
		serverf, server := startUDPServer(t, proto)
		t.Cleanup(func() { _ = serverf.Close() })
		clientf, client := startUDPClient(t, proto, server)
		t.Cleanup(func() { clientf.Close() })

		expectedPorts = append(expectedPorts, uint16(server.Port))
		unexpectedPorts = append(unexpectedPorts, uint16(client.Port))
	}

	startTCP("tcp4")
	startTCP("tcp6")
	startUDP("udp4")
	startUDP("udp6")

	serviceMap := getServicesMap(t, url)
	pid := os.Getpid()
	require.Contains(t, serviceMap, pid)
	for _, port := range expectedPorts {
		assert.Contains(t, serviceMap[pid].Ports, port)
	}
	for _, port := range unexpectedPorts {
		assert.NotContains(t, serviceMap[pid].Ports, port)
	}
}

func TestServiceName(t *testing.T) {
	url := setupDiscoveryModule(t)

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
	cmd.Env = append(cmd.Env, "DD_SERVICE=foobar")
	cmd.Env = append(cmd.Env, "YET_OTHER_ENV=test")
	err = cmd.Start()
	require.NoError(t, err)
	f.Close()

	pid := cmd.Process.Pid
	// Eventually to give the processes time to start
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		portMap := getServicesMap(t, url)
		assert.Contains(collect, portMap, pid)
		assert.Equal(t, "foobar", portMap[pid].DDService)
		assert.Equal(t, portMap[pid].DDService, portMap[pid].Name)
		assert.Equal(t, "sleep", portMap[pid].GeneratedName)
		assert.False(t, portMap[pid].DDServiceInjected)
	}, 30*time.Second, 100*time.Millisecond)
}

func TestInjectedServiceName(t *testing.T) {
	url := setupDiscoveryModule(t)

	createEnvsMemfd(t, []string{
		"OTHER_ENV=test",
		"DD_SERVICE=injected-service-name",
		"DD_INJECTION_ENABLED=service_name",
		"YET_ANOTHER_ENV=test",
	})

	listener, err := net.Listen("tcp", "")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	pid := os.Getpid()
	portMap := getServicesMap(t, url)
	require.Contains(t, portMap, pid)
	require.Equal(t, "injected-service-name", portMap[pid].DDService)
	require.Equal(t, portMap[pid].DDService, portMap[pid].Name)
	// The GeneratedName can vary depending on how the tests are run, so don't
	// assert for a specific value.
	require.NotEmpty(t, portMap[pid].GeneratedName)
	require.NotEqual(t, portMap[pid].DDService, portMap[pid].GeneratedName)
	assert.True(t, portMap[pid].DDServiceInjected)
}

func TestAPMInstrumentationInjected(t *testing.T) {
	url := setupDiscoveryModule(t)

	createEnvsMemfd(t, []string{
		"DD_INJECTION_ENABLED=service_name,tracer",
	})

	listener, err := net.Listen("tcp", "")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	pid := os.Getpid()
	portMap := getServicesMap(t, url)
	require.Contains(t, portMap, pid)
	require.Equal(t, string(apm.Injected), portMap[pid].APMInstrumentation)
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

	for _, alias := range []string{"java", "node", "sshd"} {
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
	require.NoError(t, os.Chmod(script, 0755))

	commandLineArgs := append(commandWrapper, script)
	cmd := exec.Command(commandLineArgs[0], commandLineArgs[1:]...)
	// Running the binary in the background
	require.NoError(t, cmd.Start())

	var proc *process.Process
	var err error
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		proc, err = process.NewProcess(int32(cmd.Process.Pid))
		assert.NoError(collect, err)
	}, 10*time.Second, 100*time.Millisecond)

	cmdline, err := proc.Cmdline()
	require.NoError(t, err)
	// If we wrap the script with `sh -c`, we can have differences between a local run and a kmt run, as for
	// kmt `sh` is symbolic link to bash, while locally it can be a symbolic link to dash. In the dash case, we will
	// see 2 processes `sh -c script.py` and a sub-process `python3 script.py`, while in the bash case we will see
	// only `python3 script.py`. We need to check for the command line arguments of the process to make sure we
	// are looking at the right process.
	if cmdline == strings.Join(commandLineArgs, " ") && len(commandWrapper) > 0 {
		var children []*process.Process
		require.EventuallyWithT(t, func(collect *assert.CollectT) {
			children, err = proc.Children()
			assert.NoError(collect, err)
			assert.Len(collect, children, 1)
		}, 10*time.Second, 100*time.Millisecond)
		proc = children[0]
	}
	t.Cleanup(func() { _ = proc.Kill() })

	url := setupDiscoveryModule(t)
	pid := int(proc.Pid)
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		svcMap := getServicesMap(t, url)
		assert.Contains(collect, svcMap, pid)
		assert.True(collect, validator(svcMap[pid]))
	}, 30*time.Second, 100*time.Millisecond)
}

func TestAPMInstrumentationProvided(t *testing.T) {
	curDir, err := testutil.CurDir()
	assert.NoError(t, err)

	testCases := map[string]struct {
		commandline []string // The command line of the fake server
		language    language.Language
	}{
		"java": {
			commandline: []string{"java", "-javaagent:/path/to/dd-java-agent.jar", "-jar", "foo.jar"},
			language:    language.Java,
		},
		"node": {
			commandline: []string{"node", filepath.Join(curDir, "testdata", "server.js")},
			language:    language.Node,
		},
	}

	serverDir := buildFakeServer(t)
	url := setupDiscoveryModule(t)

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(func() { cancel() })

			bin := filepath.Join(serverDir, test.commandline[0])
			cmd := exec.CommandContext(ctx, bin, test.commandline[1:]...)
			err := cmd.Start()
			require.NoError(t, err)

			pid := cmd.Process.Pid

			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				portMap := getServicesMap(t, url)
				assert.Contains(collect, portMap, pid)
				assert.Equal(collect, string(test.language), portMap[pid].Language)
				assert.Equal(collect, string(apm.Provided), portMap[pid].APMInstrumentation)
				assertStat(t, portMap[pid])
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

	assert.Equal(t, uint64(createTimeMs/1000), svc.StartTimeSecs)
}

func TestCommandLineSanitization(t *testing.T) {
	serverDir := buildFakeServer(t)
	url := setupDiscoveryModule(t)

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
		svcMap := getServicesMap(t, url)
		assert.Contains(collect, svcMap, pid)
		assert.Equal(collect, sanitizedCommandLine, svcMap[pid].CommandLine)
	}, 30*time.Second, 100*time.Millisecond)
}

func TestIgnore(t *testing.T) {
	serverDir := buildFakeServer(t)
	url := setupDiscoveryModule(t)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	badBin := filepath.Join(serverDir, "sshd")
	badCmd := exec.CommandContext(ctx, badBin)
	require.NoError(t, badCmd.Start())

	// Also run a non-ignored server so that we can use it in the eventually
	// loop below so that we don't have to wait a long time to be sure that we
	// really ignored badBin and just didn't miss it because of a race.
	goodBin := filepath.Join(serverDir, "node")
	goodCmd := exec.CommandContext(ctx, goodBin)
	require.NoError(t, goodCmd.Start())

	goodPid := goodCmd.Process.Pid
	badPid := badCmd.Process.Pid

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		svcMap := getServicesMap(t, url)
		assert.Contains(collect, svcMap, goodPid)
		require.NotContains(t, svcMap, badPid)
	}, 30*time.Second, 100*time.Millisecond)
}

func TestIgnoreCommsLengths(t *testing.T) {
	for comm := range ignoreComms {
		// /proc/PID/comm is limited to 16 characters.
		assert.LessOrEqual(t, len(comm), 16, "Process name %q too big", comm)
	}
}

func TestNodeDocker(t *testing.T) {
	cert, key, err := testutil.GetCertsPaths()
	require.NoError(t, err)

	require.NoError(t, nodejs.RunServerNodeJS(t, key, cert, "4444"))
	nodeJSPID, err := nodejs.GetNodeJSDockerPID()
	require.NoError(t, err)

	url := setupDiscoveryModule(t)
	pid := int(nodeJSPID)

	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		svcMap := getServicesMap(t, url)
		assert.Contains(collect, svcMap, pid)
		assert.Equal(collect, "nodejs-https-server", svcMap[pid].GeneratedName)
		assert.Equal(collect, svcMap[pid].GeneratedName, svcMap[pid].Name)
		assert.Equal(collect, "provided", svcMap[pid].APMInstrumentation)
		assertStat(collect, svcMap[pid])
	}, 30*time.Second, 100*time.Millisecond)
}

func TestAPMInstrumentationProvidedPython(t *testing.T) {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	fmapper := fileopener.BuildFmapper(t)
	fakePython := makeAlias(t, "python", fmapper)

	// We need the process to map something in a directory called
	// "site-packages/ddtrace". The actual mapped file does not matter.
	ddtrace := filepath.Join(curDir, "..", "..", "..", "..", "network", "usm", "testdata", "site-packages", "ddtrace")
	lib := filepath.Join(ddtrace, fmt.Sprintf("libssl.so.%s", runtime.GOARCH))

	// Give the process a listening socket
	listener, err := net.Listen("tcp", "")
	require.NoError(t, err)
	f, err := listener.(*net.TCPListener).File()
	listener.Close()
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	disableCloseOnExec(t, f)

	cmd, err := fileopener.OpenFromProcess(t, fakePython, lib)
	require.NoError(t, err)

	url := setupDiscoveryModule(t)

	pid := cmd.Process.Pid
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		portMap := getServicesMap(t, url)
		assert.Contains(collect, portMap, pid)
		assert.Equal(collect, string(language.Python), portMap[pid].Language)
		assert.Equal(collect, string(apm.Provided), portMap[pid].APMInstrumentation)
		assertStat(collect, portMap[pid])
	}, 30*time.Second, 100*time.Millisecond)
}

// Check that we can get listening processes in other namespaces.
func TestNamespaces(t *testing.T) {
	url := setupDiscoveryModule(t)

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

	// Eventually to give the processes time to start
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		portMap := getServicesMap(t, url)
		for _, pid := range pids {
			assert.Contains(collect, portMap, pid)
		}
	}, 30*time.Second, 100*time.Millisecond)

	serviceMap := getServicesMap(t, url)
	for _, pid := range pids {
		require.Contains(t, serviceMap[pid].Ports, uint16(expectedPorts[pid]))
	}
}

// Check that we are able to find services inside Docker containers.
func TestDocker(t *testing.T) {
	url := setupDiscoveryModule(t)

	dir, _ := testutil.CurDir()
	err := protocolUtils.RunDockerServer(t, "foo-server",
		dir+"/testdata/docker-compose.yml", []string{},
		regexp.MustCompile("Serving.*"),
		protocolUtils.DefaultTimeout, 3)
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

	portMap := getServicesMap(t, url)

	require.Contains(t, portMap, pid1111)
	require.Contains(t, portMap[pid1111].Ports, uint16(1234))
}

// Check that the cache is cleaned when procceses die.
func TestCache(t *testing.T) {
	wmeta := fxutil.Test[workloadmeta.Component](t,
		core.MockBundle(),
		wmmock.MockModule(workloadmeta.NewParams()),
	)
	deps := module.FactoryDependencies{
		WMeta: wmeta,
	}
	module, err := NewDiscoveryModule(nil, deps)
	require.NoError(t, err)
	discovery := module.(*discovery)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	f, _ := startTCPServer(t, "tcp4")
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
		_, err = discovery.getServices()
		require.NoError(t, err)

		for _, cmd := range cmds {
			pid := int32(cmd.Process.Pid)
			assert.Contains(collect, discovery.cache, pid)
		}
	}, 10*time.Second, 100*time.Millisecond)

	for i, cmd := range cmds {
		pid := int32(cmd.Process.Pid)
		require.Equal(t, serviceNames[i], discovery.cache[pid].ddServiceName)
		require.False(t, discovery.cache[pid].ddServiceInjected)
	}

	cancel()
	for _, cmd := range cmds {
		cmd.Wait()
	}

	_, err = discovery.getServices()
	require.NoError(t, err)

	for _, cmd := range cmds {
		pid := cmd.Process.Pid
		require.NotContains(t, discovery.cache, int32(pid))
	}

	discovery.Close()
	require.Empty(t, discovery.cache)
}

func BenchmarkOldProcess(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		process.NewProcess(int32(os.Getpid()))
	}
}

func BenchmarkNewProcess(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		customNewProcess(int32(os.Getpid()))
	}
}

func getSocketsOld(p *process.Process) ([]uint64, error) {
	FDs, err := p.OpenFiles()
	if err != nil {
		return nil, err
	}

	// sockets have the following pattern "socket:[inode]"
	var sockets []uint64
	for _, fd := range FDs {
		if strings.HasPrefix(fd.Path, prefix) {
			inodeStr := strings.TrimPrefix(fd.Path[:len(fd.Path)-1], prefix)
			sock, err := strconv.ParseUint(inodeStr, 10, 64)
			if err != nil {
				continue
			}
			sockets = append(sockets, sock)
		}
	}

	return sockets, nil
}

const (
	numberFDs = 100
)

func createFilesAndSockets(tb testing.TB) {
	listeningSockets := make([]net.Listener, 0, numberFDs)
	tb.Cleanup(func() {
		for _, l := range listeningSockets {
			l.Close()
		}
	})
	for i := 0; i < numberFDs; i++ {
		l, err := net.Listen("tcp", "localhost:0")
		require.NoError(tb, err)
		listeningSockets = append(listeningSockets, l)
	}
	regularFDs := make([]*os.File, 0, numberFDs)
	tb.Cleanup(func() {
		for _, f := range regularFDs {
			f.Close()
		}
	})
	for i := 0; i < numberFDs; i++ {
		f, err := os.CreateTemp("", "")
		require.NoError(tb, err)
		regularFDs = append(regularFDs, f)
	}
}

func TestGetSockets(t *testing.T) {
	createFilesAndSockets(t)
	p, err := process.NewProcess(int32(os.Getpid()))
	require.NoError(t, err)

	sockets, err := getSockets(p.Pid)
	require.NoError(t, err)

	sockets2, err := getSocketsOld(p)
	require.NoError(t, err)

	require.Equal(t, sockets, sockets2)
}

func BenchmarkGetSockets(b *testing.B) {
	createFilesAndSockets(b)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		getSockets(int32(os.Getpid()))
	}
}

func BenchmarkOldGetSockets(b *testing.B) {
	createFilesAndSockets(b)
	p, err := process.NewProcess(int32(os.Getpid()))
	require.NoError(b, err)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		getSocketsOld(p)
	}
}

// addSockets adds only listening sockets to a map to be used for later looksups.
func addSockets[P procfs.NetTCP | procfs.NetUDP](sockMap map[uint64]socketInfo, sockets P, state uint64) {
	for _, sock := range sockets {
		if sock.St != state {
			continue
		}
		sockMap[sock.Inode] = socketInfo{port: uint16(sock.LocalPort)}
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

	addSockets(listeningSockets, TCP, tcpListen)
	addSockets(listeningSockets, TCP6, tcpListen)
	addSockets(listeningSockets, UDP, udpListen)
	addSockets(listeningSockets, UDP6, udpListen)

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
