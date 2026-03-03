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
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"

	gorillamux "github.com/gorilla/mux"
	"github.com/prometheus/procfs"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/language"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

func findService(pid int, services []model.Service) *model.Service {
	for _, s := range services {
		if s.PID == pid {
			return &s
		}
	}

	return nil
}

type testDiscoveryModule struct {
	url string
}

func setupDiscoveryModule(t *testing.T) *testDiscoveryModule {
	t.Helper()
	mux := gorillamux.NewRouter()

	mod, err := NewDiscoveryModule(nil, module.FactoryDependencies{})
	require.NoError(t, err)
	discovery := mod.(*discovery)

	discovery.Register(module.NewRouter(string(config.DiscoveryModule), mux))
	t.Cleanup(discovery.Close)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &testDiscoveryModule{
		url: srv.URL,
	}
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
		req, err = http.NewRequest(http.MethodPost, url, body)
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(http.MethodPost, url, nil)
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
func getRunningPids(t require.TestingT) []int32 {
	pids, err := process.Pids()
	require.NoError(t, err)
	return pids
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

	// Disable close-on-exec so that the process gets it
	t.Cleanup(func() { f.Close() })
	disableCloseOnExec(t, f)

	cmd := exec.CommandContext(ctx, "sleep", "1000")
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
	err := cmd.Start()
	require.NoError(t, err)
	f.Close()

	return cmd
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

func newDiscovery() *discovery {
	mod, err := NewDiscoveryModule(nil, module.FactoryDependencies{})
	if err != nil {
		panic(err)
	}
	return mod.(*discovery)
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
	path := kernel.HostProc(strconv.Itoa(pid))
	proc, err := procfs.NewFS(path)
	if err != nil {
		return nil, err
	}

	TCP, _ := proc.NetTCP()
	UDP, _ := proc.NetUDP()
	TCP6, _ := proc.NetTCP6()
	UDP6, _ := proc.NetUDP6()

	tcpSockets := make(map[uint64]socketInfo)
	udpSockets := make(map[uint64]socketInfo)

	addSockets(tcpSockets, TCP, network.AFINET, network.TCP, tcpListen)
	addSockets(tcpSockets, TCP6, network.AFINET6, network.TCP, tcpListen)
	addSockets(udpSockets, UDP, network.AFINET, network.UDP, udpListen)
	addSockets(udpSockets, UDP6, network.AFINET6, network.UDP, udpListen)

	return &namespaceInfo{
		tcpSockets: tcpSockets,
		udpSockets: udpSockets,
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
	discovery := newDiscovery()
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

		buf := make([]byte, readlinkBufferSize)
		openFiles, err := getOpenFilesInfo(int32(self), buf)
		require.NoError(t, err)

		info, err := discovery.getServiceInfo(int32(self), openFiles)
		require.NoError(t, err)
		require.Equal(t, language.CPlusPlus, language.Language(info.Language))
		require.Equal(t, true, info.APMInstrumentation)
	})

	t.Run("invalid metadata", func(t *testing.T) {
		createTracerMemfd(t, []byte("invalid data"))

		buf := make([]byte, readlinkBufferSize)
		openFiles, err := getOpenFilesInfo(int32(self), buf)
		require.NoError(t, err)

		info, err := discovery.getServiceInfo(int32(self), openFiles)
		require.NoError(t, err)
		require.Equal(t, false, info.APMInstrumentation)
	})
}

func TestDetectAPMInjectorFromMaps(t *testing.T) {
	tests := []struct {
		name     string
		maps     string
		expected bool
	}{
		{
			name:     "empty maps",
			maps:     "",
			expected: false,
		},
		{
			name: "no injector in maps",
			maps: `aaaacd3c0000-aaaacd49e000 r-xp 00000000 00:22 25173                      /usr/bin/bash
aaaacd4ac000-aaaacd4b0000 r--p 000ec000 00:22 25173                      /usr/bin/bash
aaaacd4b0000-aaaacd4b4000 rw-p 000f0000 00:22 25173                      /usr/bin/bash
ffffb7360000-ffffb74ec000 r-xp 00000000 00:22 13920                      /usr/lib64/libc.so.6
ffffb74ec000-ffffb74fd000 ---p 0018c000 00:22 13920                      /usr/lib64/libc.so.6`,
			expected: false,
		},
		{
			name: "injector present",
			maps: `aaaacd3c0000-aaaacd49e000 r-xp 00000000 00:22 25173                      /usr/bin/bash
aaaacd4ac000-aaaacd4b0000 r--p 000ec000 00:22 25173                      /usr/bin/bash
ffffb7360000-ffffb74ec000 r-xp 00000000 00:22 13920                      /opt/datadog-packages/datadog-apm-inject/1.0.0/inject/launcher.preload.so
ffffb74ec000-ffffb74fd000 ---p 0018c000 00:22 13920                      /usr/lib64/libc.so.6`,
			expected: true,
		},
		{
			name: "injector with different version",
			maps: `aaaacd3c0000-aaaacd49e000 r-xp 00000000 00:22 25173                      /usr/bin/bash
ffffb7360000-ffffb74ec000 r-xp 00000000 00:22 13920                      /opt/datadog-packages/datadog-apm-inject/2.5.3-beta/inject/launcher.preload.so`,
			expected: true,
		},
		{
			name: "similar but not matching paths",
			maps: `aaaacd3c0000-aaaacd49e000 r-xp 00000000 00:22 25173                      /opt/datadog-packages/datadog-apm-inject/1.0.0/launcher.preload.so
aaaacd4ac000-aaaacd4b0000 r--p 000ec000 00:22 25173                      /opt/datadog-packages/datadog-apm-inject/1.0.0/inject/launcher.so
ffffb7360000-ffffb74ec000 r-xp 00000000 00:22 13920                      /opt/other-packages/datadog-apm-inject/1.0.0/inject/launcher.preload.so`,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectAPMInjectorFromMapsReader(strings.NewReader(tt.maps))
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestRustBinary(t *testing.T) {
	// Skip on CentOS 7 due to Rust binary not being statically linked
	platform, err := kernel.Platform()
	require.NoError(t, err)
	platformVersion, err := kernel.PlatformVersion()
	require.NoError(t, err)

	if platform == "centos" && strings.HasPrefix(platformVersion, "7") {
		t.Skip("Skipping Rust binary test on CentOS 7 due to glibc compatibility issues with non-static binary")
	}

	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	binaryPath := filepath.Join(curDir, "rust", "sd-agent")

	require.FileExists(t, binaryPath, "Rust binary should be built")

	truePath := "/bin/true"
	if _, err := os.Stat(truePath); os.IsNotExist(err) {
		truePath = "/usr/bin/true"
	}

	env := os.Environ()
	env = append(env, "DD_DISCOVERY_USE_SD_AGENT=true")
	env = append(env, "DD_DISCOVERY_ENABLED=false")
	// Fake system-probe binary with empty configuration file
	cmd := exec.Command(binaryPath, "--", truePath, "-c", "/dev/null")
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Rust binary should execute successfully")
	require.Contains(t, string(output), "Discovery is disabled")
	require.Equal(t, 0, cmd.ProcessState.ExitCode(), "Binary should exit with code 0", string(output))
}
