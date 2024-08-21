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
	"syscall"
	"testing"
	"time"

	"net/http/httptest"

	gorillamux "github.com/gorilla/mux"
	"github.com/prometheus/procfs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netns"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/apm"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/model"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	protocolUtils "github.com/DataDog/datadog-agent/pkg/network/protocols/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func setupDiscoveryModule(t *testing.T) string {
	t.Helper()

	wmeta := optional.NewNoneOption[workloadmeta.Component]()
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
		assert.Equal(t, "foobar", portMap[pid].Name)
	}, 30*time.Second, 100*time.Millisecond)
}

func TestInjectedServiceName(t *testing.T) {
	url := setupDiscoveryModule(t)

	createEnvsMemfd(t, []string{
		"OTHER_ENV=test",
		"DD_SERVICE=injected-service-name",
		"YET_ANOTHER_ENV=test",
	})

	listener, err := net.Listen("tcp", "")
	require.NoError(t, err)
	t.Cleanup(func() { listener.Close() })

	pid := os.Getpid()
	portMap := getServicesMap(t, url)
	require.Contains(t, portMap, pid)
	require.Equal(t, "injected-service-name", portMap[pid].Name)
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

func buildFakeServer(t *testing.T) string {
	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	serverBin, err := usmtestutil.BuildGoBinaryWrapper(filepath.Join(curDir, "testutil"), "fake_server")
	require.NoError(t, err)

	binDir := filepath.Dir(serverBin)
	for _, alias := range []string{"java", "node"} {
		aliasPath := filepath.Join(binDir, alias)

		target, err := os.Readlink(aliasPath)
		if err == nil && target == serverBin {
			continue
		}

		os.Remove(aliasPath)
		err = os.Symlink(serverBin, aliasPath)
		require.NoError(t, err)
	}

	return binDir
}

func TestAPMInstrumentationProvided(t *testing.T) {
	curDir, err := testutil.CurDir()
	assert.NoError(t, err)

	testCases := map[string]struct {
		commandline      []string // The command line of the fake server
		workingDirectory string   // Optional: The working directory to use for the server.
	}{
		"java": {
			commandline: []string{"java", "-javaagent:/path/to/dd-java-agent.jar", "-jar", "foo.jar"},
		},
		"node": {
			commandline:      []string{"node"},
			workingDirectory: filepath.Join(curDir, "testdata"),
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
			cmd.Dir = test.workingDirectory
			err := cmd.Start()
			require.NoError(t, err)

			pid := cmd.Process.Pid

			require.EventuallyWithT(t, func(collect *assert.CollectT) {
				portMap := getServicesMap(t, url)
				assert.Contains(collect, portMap, pid)
				assert.Equal(collect, string(apm.Provided), portMap[pid].APMInstrumentation)
			}, 30*time.Second, 100*time.Millisecond)
		})
	}
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
	module, err := NewDiscoveryModule(nil, optional.NewNoneOption[workloadmeta.Component](), nil)
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
		require.Contains(t, discovery.cache[pid].name, serviceNames[i])
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
