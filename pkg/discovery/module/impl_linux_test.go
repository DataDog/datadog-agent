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
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/discovery/core"
	"github.com/DataDog/datadog-agent/pkg/discovery/model"
	"github.com/DataDog/datadog-agent/pkg/discovery/module/splite"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	usmtestutil "github.com/DataDog/datadog-agent/pkg/network/usm/testutil"
	spclient "github.com/DataDog/datadog-agent/pkg/system-probe/api/client"
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
	url    string
	client *http.Client
}

func setupRustLibraryDiscoveryModule(t *testing.T) *testDiscoveryModule {
	t.Helper()

	mux := http.NewServeMux()

	mod, err := NewDiscoveryModule(nil, module.FactoryDependencies{})
	require.NoError(t, err)
	discovery := mod.(*discovery)

	discovery.Register(module.NewRouter(string(config.DiscoveryModule), mux))
	t.Cleanup(discovery.Close)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return &testDiscoveryModule{
		url:    srv.URL,
		client: http.DefaultClient,
	}
}

func setupRustDiscoveryModule(t *testing.T) *testDiscoveryModule {
	t.Helper()

	// CentOS 7 arm64 is not a supported platform (arm64 support starts at CentOS 8)
	// and the binary requires GLIBC_2.18 which is not available on CentOS 7 (glibc 2.17).
	if runtime.GOARCH == "arm64" {
		platform, err := kernel.Platform()
		require.NoError(t, err)
		platformVersion, err := kernel.PlatformVersion()
		require.NoError(t, err)
		if platform == "centos" && strings.HasPrefix(platformVersion, "7") {
			t.Skip("system-probe-lite requires GLIBC_2.18 on arm64; CentOS 7 (glibc 2.17) is unsupported on arm64")
		}
	}

	curDir, err := testutil.CurDir()
	require.NoError(t, err)
	binaryPath := filepath.Join(curDir, "rust", "embedded", "bin", "system-probe-lite")
	require.FileExists(t, binaryPath, "system-probe-lite binary should be built")

	socketDir := t.TempDir()
	socketPath := filepath.Join(socketDir, "sysprobe.sock")

	cfg := &splite.Config{Socket: socketPath}

	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, binaryPath, cfg.Args()...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})

	// Dial rather than stat: the socket file becomes visible after bind(2) but
	// before listen(2), so a stale poll can win that window and the subsequent
	// HTTP request fails with ECONNREFUSED.
	require.Eventually(t, func() bool {
		conn, err := net.Dial("unix", socketPath)
		if err != nil {
			return false
		}
		conn.Close()
		return true
	}, 10*time.Second, 50*time.Millisecond, "system-probe-lite socket did not become ready")

	return &testDiscoveryModule{
		url: "http://sysprobe",
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				DialContext: spclient.DialContextFunc(socketPath),
			},
		},
	}
}

type discoveryTestSuite struct {
	suite.Suite
	setupModule            func(t *testing.T) *testDiscoveryModule
	discovery              *testDiscoveryModule
	expectedImplementation string
}

func (s *discoveryTestSuite) SetupTest() {
	s.discovery = s.setupModule(s.T())
}

func (s *discoveryTestSuite) TestState() {
	t := s.T()

	url := s.discovery.url + "/" + string(config.DiscoveryModule) + "/state"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)

	resp, err := s.discovery.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var state map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&state)
	require.NoError(t, err)

	require.Equal(t, s.expectedImplementation, state["implementation"])
}

func TestDiscovery(t *testing.T) {
	t.Run("rust", func(t *testing.T) {
		suite.Run(t, &discoveryTestSuite{setupModule: setupRustDiscoveryModule, expectedImplementation: "system-probe-lite"})
	})
	t.Run("rust-library", func(t *testing.T) {
		suite.Run(t, &discoveryTestSuite{setupModule: setupRustLibraryDiscoveryModule, expectedImplementation: "system-probe"})
	})
}

// makeRequest wraps the request to the discovery module, setting the JSON body if provided,
// and returning the response as the given type.
func makeRequest[T any](t require.TestingT, client *http.Client, url string, params *core.Params) *T {
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

	resp, err := client.Do(req)
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

func setMemfdMtime(t *testing.T, fd int, mtime time.Time) {
	t.Helper()
	path := fmt.Sprintf("/proc/self/fd/%d", fd)
	ts := []unix.Timespec{
		unix.NsecToTimespec(mtime.UnixNano()),
		unix.NsecToTimespec(mtime.UnixNano()),
	}
	err := unix.UtimesNanoAt(unix.AT_FDCWD, path, ts, 0)
	require.NoError(t, err)

	// Read back and verify the timestamp was applied.
	var stat unix.Stat_t
	err = unix.Stat(path, &stat)
	require.NoError(t, err)
	got := time.Unix(stat.Mtim.Sec, stat.Mtim.Nsec)
	require.Equalf(t, mtime.UnixNano(), got.UnixNano(), "memfd mtime was not set correctly: want %v (%d), got %v (%d)", mtime, mtime.UnixNano(), got, got.UnixNano())
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
