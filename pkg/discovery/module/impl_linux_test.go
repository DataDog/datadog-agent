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
	env = append(env, "DD_DISCOVERY_ENABLED=false")
	// Fake system-probe binary with empty configuration file
	cmd := exec.Command(binaryPath, "--", truePath, "-c", "/dev/null")
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "Rust binary should execute successfully")
	require.Contains(t, string(output), "Discovery is disabled")
	require.Equal(t, 0, cmd.ProcessState.ExitCode(), "Binary should exit with code 0", string(output))
}

func TestParseHexIP(t *testing.T) {
	tests := []struct {
		name           string
		hexIP          string
		inputFamily    string
		expectedIP     string
		expectedFamily string
		expectError    bool
	}{
		{
			name:           "IPv6-mapped IPv4 address (10.244.1.11)",
			hexIP:          "0000000000000000FFFF00000B01F40A",
			inputFamily:    "v6",
			expectedIP:     "10.244.1.11",
			expectedFamily: "v4",
			expectError:    false,
		},
		{
			name:           "IPv6-mapped IPv4 address (10.244.1.12)",
			hexIP:          "0000000000000000FFFF00000C01F40A",
			inputFamily:    "v6",
			expectedIP:     "10.244.1.12",
			expectedFamily: "v4",
			expectError:    false,
		},
		{
			name:           "Regular IPv6 address (2001:db8::1)",
			hexIP:          "B80D0120000000000000000001000000",
			inputFamily:    "v6",
			expectedIP:     "2001:db8::1",
			expectedFamily: "v6",
			expectError:    false,
		},
		{
			name:           "Regular IPv6 address (fe80::1)",
			hexIP:          "000080FE000000000000000001000000",
			inputFamily:    "v6",
			expectedIP:     "fe80::1",
			expectedFamily: "v6",
			expectError:    false,
		},
		{
			name:           "Plain IPv4 address (10.244.1.11)",
			hexIP:          "0B01F40A",
			inputFamily:    "v4",
			expectedIP:     "10.244.1.11",
			expectedFamily: "v4",
			expectError:    false,
		},
		{
			name:           "Plain IPv4 address (192.168.1.1)",
			hexIP:          "0101A8C0",
			inputFamily:    "v4",
			expectedIP:     "192.168.1.1",
			expectedFamily: "v4",
			expectError:    false,
		},
		{
			name:           "IPv6 zero address (::)",
			hexIP:          "00000000000000000000000000000000",
			inputFamily:    "v6",
			expectedIP:     "::",
			expectedFamily: "v6",
			expectError:    false,
		},
		{
			name:           "IPv4 zero address (0.0.0.0)",
			hexIP:          "00000000",
			inputFamily:    "v4",
			expectedIP:     "0.0.0.0",
			expectedFamily: "v4",
			expectError:    false,
		},
		{
			name:        "Invalid IPv6 length",
			hexIP:       "0000000000000000FFFF00000B01F4",
			inputFamily: "v6",
			expectError: true,
		},
		{
			name:        "Invalid IPv4 length",
			hexIP:       "0B01F4",
			inputFamily: "v4",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip, family, err := parseHexIPBytes([]byte(tt.hexIP), tt.inputFamily)
			if tt.expectError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.expectedIP, ip, "IP address mismatch")
			require.Equal(t, tt.expectedFamily, family, "Family mismatch")
		})
	}
}

func TestParseEstablishedConnLineIPv6Mapped(t *testing.T) {
	// Test that processNetTCPLine correctly normalizes IPv6-mapped IPv4 addresses
	// This simulates a line from /proc/net/tcp6 with IPv6-mapped IPv4 addresses
	// Example: cluster-agent listening on 10.244.1.11:5005, agent connecting from 10.244.1.12:46538
	// Line format: sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode ...
	line := []byte("   0: 0000000000000000FFFF00000B01F40A:138D 0000000000000000FFFF00000C01F40A:B5CA 01 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000 100 0 0 10 0")

	listening := make(map[uint64]uint16)
	established := make(map[uint64]*establishedConnInfo)

	processNetTCPLine(line, "v6", listening, established)

	require.Empty(t, listening, "Should not extract listening socket from established connection")
	require.Len(t, established, 1, "Should extract one established connection")

	connInfo, ok := established[12345]
	require.True(t, ok, "Connection should be keyed by inode 12345")
	require.NotNil(t, connInfo)

	// Verify that IPv6-mapped addresses are normalized to plain IPv4
	require.Equal(t, "10.244.1.11", connInfo.localIP, "Local IP should be normalized to plain IPv4")
	require.Equal(t, uint16(5005), connInfo.localPort)
	require.Equal(t, "10.244.1.12", connInfo.remoteIP, "Remote IP should be normalized to plain IPv4")
	require.Equal(t, uint16(46538), connInfo.remotePort)
	require.Equal(t, "v4", connInfo.family, "Family should be v4 for IPv6-mapped IPv4 addresses")
}

func TestParseNetTCPComplete(t *testing.T) {
	// Test that parseNetTCPComplete correctly extracts both listening and established
	// connections by parsing the files once.
	// We'll create a test process and verify the parsing works correctly.

	// Start a TCP server (listening socket)
	serverFile, serverAddr := startTCPServer(t, "tcp", "127.0.0.1:0")
	defer serverFile.Close()

	// Start a TCP client (established connection)
	clientFile, clientAddr := startTCPClient(t, "tcp", serverAddr)
	defer clientFile.Close()

	// Parse the current process's network namespace
	completeInfo, err := parseNetTCPComplete(os.Getpid())
	require.NoError(t, err)
	require.NotNil(t, completeInfo)
	require.NotNil(t, completeInfo.listening)
	require.NotNil(t, completeInfo.established)

	// Get the socket inodes for our test sockets
	serverStat, err := serverFile.Stat()
	require.NoError(t, err)
	serverInode := serverStat.Sys().(*syscall.Stat_t).Ino

	clientStat, err := clientFile.Stat()
	require.NoError(t, err)
	clientInode := clientStat.Sys().(*syscall.Stat_t).Ino

	// Verify the listening socket is in the tcpSockets map
	serverInfo, ok := completeInfo.listening.tcpSockets[serverInode]
	require.True(t, ok, "Server socket should be in listening sockets map")
	require.Equal(t, uint16(serverAddr.Port), serverInfo.port, "Server port should match")

	// Verify the established connection is in the established map
	// The client connection should be in the v4 established connections
	clientConnInfo, ok := completeInfo.established.v4[clientInode]
	require.True(t, ok, "Client socket should be in established connections map")
	require.Equal(t, "127.0.0.1", clientConnInfo.localIP, "Client local IP should be 127.0.0.1")
	require.Equal(t, uint16(clientAddr.Port), clientConnInfo.localPort, "Client local port should match")
	require.Equal(t, "127.0.0.1", clientConnInfo.remoteIP, "Client remote IP should be 127.0.0.1")
	require.Equal(t, uint16(serverAddr.Port), clientConnInfo.remotePort, "Client remote port should match server port")
	require.Equal(t, "v4", clientConnInfo.family, "Connection family should be v4")
}

func TestProcessNetTCPLine(t *testing.T) {
	// Test that processNetTCPLine correctly handles both listening and established states
	tests := []struct {
		name              string
		line              string
		expectListening   bool
		expectEstablished bool
		expectedPort      uint16
		expectedInode     uint64
	}{
		{
			name: "listening socket",
			// State 0A = listening (10 in hex)
			line:            "   0: 0100007F:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000 100 0 0 10 0",
			expectListening: true,
			expectedPort:    8080,
			expectedInode:   12345,
		},
		{
			name: "established connection",
			// State 01 = established (1 in hex)
			line:              "   0: 0100007F:1F91 0100007F:1F90 01 00000000:00000000 00:00000000 00000000  1000        0 54321 1 0000000000000000 100 0 0 10 0",
			expectEstablished: true,
			expectedInode:     54321,
		},
		{
			name: "other state (should be ignored)",
			// State 02 = SYN_SENT
			line:              "   0: 0100007F:1F91 0100007F:1F90 02 00000000:00000000 00:00000000 00000000  1000        0 99999 1 0000000000000000 100 0 0 10 0",
			expectListening:   false,
			expectEstablished: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listening := make(map[uint64]uint16)
			established := make(map[uint64]*establishedConnInfo)

			processNetTCPLine([]byte(tt.line), "v4", listening, established)

			if tt.expectListening {
				port, ok := listening[tt.expectedInode]
				require.True(t, ok, "Expected inode should be in listening map")
				require.Equal(t, tt.expectedPort, port, "Port should match")
			} else {
				require.Empty(t, listening, "Listening map should be empty")
			}

			if tt.expectEstablished {
				connInfo, ok := established[tt.expectedInode]
				require.True(t, ok, "Expected inode should be in established map")
				require.NotNil(t, connInfo)
				require.Equal(t, "127.0.0.1", connInfo.localIP)
			} else {
				require.Empty(t, established, "Established map should be empty")
			}
		})
	}
}
