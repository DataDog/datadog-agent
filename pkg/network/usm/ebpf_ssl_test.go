// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package usm

import (
	"context"
	"errors"
	"net"
	nethttp "net/http"
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
	"unsafe"

	manager "github.com/DataDog/ebpf-manager"
	"github.com/cilium/ebpf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/network/protocols"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http"
	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/consts"
	fileopener "github.com/DataDog/datadog-agent/pkg/network/usm/sharedlibraries/testutil"
	"github.com/DataDog/datadog-agent/pkg/network/usm/utils"
	globalutils "github.com/DataDog/datadog-agent/pkg/util/testutil"
	dockerutils "github.com/DataDog/datadog-agent/pkg/util/testutil/docker"
)

const (
	addressOfHTTPPythonServer = "127.0.0.1:8001"
)

// setNativeTLSPeriodicTerminatedProcessesScanInterval sets the interval for the periodic scan of terminated processes in GoTLS.
func setNativeTLSPeriodicTerminatedProcessesScanInterval(tb testing.TB, interval time.Duration) {
	originalValue := nativeTLSScanTerminatedProcessesInterval
	tb.Cleanup(func() {
		nativeTLSScanTerminatedProcessesInterval = originalValue
	})
	nativeTLSScanTerminatedProcessesInterval = interval
}

func testArch(t *testing.T, arch string) {
	cfg := NewUSMEmptyConfig()
	cfg.EnableNativeTLSMonitoring = true

	utils.SkipIfTLSUnsupported(t, cfg)

	curDir, err := testutil.CurDir()
	require.NoError(t, err)

	// Named site-packages/ddtrace since it is used from servicediscovery tests too.
	libmmap := filepath.Join(curDir, "testdata", "site-packages", "ddtrace")
	lib := filepath.Join(libmmap, "libssl.so."+arch)

	monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
	require.NotNil(t, monitor)

	cmd, err := fileopener.OpenFromAnotherProcess(t, lib)
	require.NoError(t, err)

	if arch == runtime.GOARCH {
		utils.WaitForProgramsToBeTraced(t, consts.USMModuleName, UsmTLSAttacherName, cmd.Process.Pid, utils.ManualTracingFallbackDisabled)
	} else {
		utils.WaitForPathToBeBlocked(t, consts.USMModuleName, UsmTLSAttacherName, lib)
	}
}

func TestArchAmd64(t *testing.T) {
	testArch(t, "amd64")
}

func TestArchArm64(t *testing.T) {
	testArch(t, "arm64")
}

// findNonExistingPid finds a PID that doesn't exist on the system
func findNonExistingPid(t *testing.T) int {
	// Start from a high number to avoid common system PIDs
	for pid := 100000; pid < 1000000; pid++ {
		// On Linux, kill(pid, 0) returns 0 if process exists, -1 if it doesn't
		if err := syscall.Kill(pid, 0); err != nil {
			if errors.Is(err, syscall.ESRCH) { // No such process
				return pid
			}
		}
	}
	t.Log("Failed to find a non-existing PID")
	t.FailNow()
	return 0
}

// TestSSLMapsCleaner verifies that SSL-related kernel maps are cleared correctly.
// the map entry is deleted when the thread exits, also periodic map cleaner removes dead threads.
func TestSSLMapsCleaner(t *testing.T) {
	setNativeTLSPeriodicTerminatedProcessesScanInterval(t, time.Second)
	// setup monitor
	cfg := NewUSMEmptyConfig()
	cfg.EnableNativeTLSMonitoring = true
	// test cleanup is faster without event stream, this test does not require event stream
	cfg.EnableUSMEventStream = false

	utils.SkipIfTLSUnsupported(t, cfg)
	// use the monitor and its eBPF manager to check and access SSL related maps
	monitor := setupUSMTLSMonitor(t, cfg, reInitEventConsumer)
	require.NotNil(t, monitor)

	cleanProtocolMaps(t, "ssl", monitor.ebpfProgram.Manager.Manager)
	cleanProtocolMaps(t, bioNewSocketArgsMap, monitor.ebpfProgram.Manager.Manager)

	// find maps by names
	sslPidKeyMaps := []string{sslReadArgsMap, sslReadExArgsMap, sslWriteArgsMap, sslWriteExArgsMap, bioNewSocketArgsMap}
	maps := getMaps(t, monitor.ebpfProgram.Manager.Manager, sslPidKeyMaps)
	require.Equal(t, len(maps), len(sslPidKeyMaps))

	// add random pid to the maps
	pid := findNonExistingPid(t)
	addPidEntryToMaps(t, maps, pid)
	checkPidExistsInMaps(t, monitor.ebpfProgram.Manager.Manager, maps, pid)

	// verify that map is empty after cleaning up terminated processes
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		checkPidNotFoundInMaps(t, ct, monitor.ebpfProgram.Manager.Manager, maps, pid)
	}, 10*time.Second, 1*time.Second, "pid was not removed from the maps after process exit")

	// start dummy program and add its pid to the map
	cmd, cancel := startDummyProgram(t)
	addPidEntryToMaps(t, maps, cmd.Process.Pid)
	checkPidExistsInMaps(t, monitor.ebpfProgram.Manager.Manager, maps, cmd.Process.Pid)

	// verify exit of process cleans the map
	cancel()
	_ = cmd.Wait()
	checkPidNotFoundInMaps(t, t, monitor.ebpfProgram.Manager.Manager, maps, cmd.Process.Pid)
}

// getMaps returns eBPF maps searched by names.
func getMaps(t *testing.T, manager *manager.Manager, mapNames []string) []*ebpf.Map {
	maps := make([]*ebpf.Map, 0, len(mapNames))
	for _, mapName := range mapNames {
		emap, _, _ := manager.GetMap(mapName)
		require.NotNil(t, emap)
		maps = append(maps, emap)
	}
	return maps
}

// addPidEntryToMaps adds an entry to maps using the PID as a key.
func addPidEntryToMaps(t *testing.T, maps []*ebpf.Map, pid int) {
	for _, m := range maps {
		require.Equal(t, m.KeySize(), uint32(unsafe.Sizeof(uint64(0))), "wrong key size")

		// make the key for single thread process when pid and tgid are the same
		key := uint64(pid)<<32 | uint64(pid)
		value := make([]byte, m.ValueSize())

		err := m.Put(unsafe.Pointer(&key), unsafe.Pointer(&value))
		require.NoError(t, err)
	}
}

// checkPidExistsInMaps checks that pid exists in all provided maps.
func checkPidExistsInMaps(t *testing.T, manager *manager.Manager, maps []*ebpf.Map, pid int) {
	// make the key for single thread process when pid and tgid are the same
	key := uint64(pid)<<32 | uint64(pid)

	for _, m := range maps {
		require.Equal(t, m.KeySize(), uint32(unsafe.Sizeof(uint64(0))), "wrong key size")
		mapInfo, err := m.Info()
		require.NoError(t, err)

		assert.Eventually(t, func() bool {
			return findKeyInMap[uint64](m, key)
		}, 1*time.Second, 100*time.Millisecond)
		if t.Failed() {
			t.Logf("pid '%d' not found in the map %q", pid, mapInfo.Name)
			ebpftest.DumpMapsTestHelper(t, manager.DumpMaps, mapInfo.Name)
			t.FailNow()
		}
	}
}

// checkPidNotFoundInMaps checks that pid does not exist in all provided maps.
func checkPidNotFoundInMaps(originalT *testing.T, t require.TestingT, manager *manager.Manager, maps []*ebpf.Map, pid int) {
	// make the key for single thread process when pid and tgid are the same
	key := uint64(pid)<<32 | uint64(pid)

	for _, m := range maps {
		require.Equal(t, m.KeySize(), uint32(unsafe.Sizeof(uint64(0))), "wrong key size")
		mapInfo, err := m.Info()
		require.NoError(t, err)

		if findKeyInMap[uint64](m, key) {
			originalT.Logf("pid '%d' was found in the map %q", pid, mapInfo.Name)
			ebpftest.DumpMapsTestHelper(originalT, manager.DumpMaps, mapInfo.Name)
			t.FailNow()
		}
	}
}

// findKeyInMap is a generic helper to find a key in an eBPF map.
func findKeyInMap[K comparable](m *ebpf.Map, theKey K) bool {
	val := make([]byte, m.ValueSize())
	return m.Lookup(unsafe.Pointer(&theKey), unsafe.Pointer(&val[0])) == nil
}

// startDummyProgram starts sleeping thread.
func startDummyProgram(t *testing.T) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() { cancel() })

	cmd := exec.CommandContext(ctx, "sleep", "1000")
	err := cmd.Start()
	require.NoError(t, err)

	return cmd, cancel
}

// TestSSLMapsCleanup verifies that the eBPF cleanup mechanism
// correctly removes entries from the ssl_sock_by_ctx and ssl_ctx_by_tuple maps
// when the TCP connection associated with a TLS session is closed.
func TestSSLMapsCleanup(t *testing.T) {
	utils.SkipIfTLSUnsupported(t, NewUSMEmptyConfig())

	cfg := NewUSMEmptyConfig()
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableHTTPMonitoring = true
	usmMonitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)

	_ = testutil.HTTPPythonServer(t, addressOfHTTPPythonServer, testutil.Options{
		EnableTLS: true,
	})

	client, requestFn := simpleGetRequestsGenerator(t, addressOfHTTPPythonServer)
	var requests []*nethttp.Request
	for i := 0; i < numberOfRequests; i++ {
		requests = append(requests, requestFn())
	}

	sslSockMap, mapExists, errMap := usmMonitor.ebpfProgram.Manager.GetMap(sslSockByCtxMap)
	require.NoErrorf(t, errMap, "Error getting map %s", sslSockByCtxMap)
	require.Truef(t, mapExists, "Map %s does not exist on this branch. This test expects it.", sslSockByCtxMap)
	require.NotNilf(t, sslSockMap, "Map %s object is nil.", sslSockByCtxMap)

	sslTupleMap, tupleMapExists, errTupleMap := usmMonitor.ebpfProgram.Manager.GetMap(sslCtxByTupleMap)
	require.NoErrorf(t, errTupleMap, "Error getting map %s", sslCtxByTupleMap)
	require.Truef(t, tupleMapExists, "Map %s does not exist on this branch. This test expects it.", sslCtxByTupleMap)
	require.NotNilf(t, sslTupleMap, "Map %s object is nil.", sslCtxByTupleMap)

	ctxMapCountBefore := utils.CountMapEntries(t, sslSockMap)
	tupleMapCountBefore := utils.CountMapEntries(t, sslTupleMap)
	t.Logf("Count for map '%s' BEFORE CloseIdleConnections(): %d", sslSockByCtxMap, ctxMapCountBefore)
	t.Logf("Count for map '%s' BEFORE CloseIdleConnections(): %d", sslCtxByTupleMap, tupleMapCountBefore)

	client.CloseIdleConnections()

	time.Sleep(1 * time.Second)

	ctxMapCountAfter := utils.CountMapEntries(t, sslSockMap)
	tupleMapCountAfter := utils.CountMapEntries(t, sslTupleMap)
	t.Logf("Count for map '%s' AFTER CloseIdleConnections(): %d", sslSockByCtxMap, ctxMapCountAfter)
	t.Logf("Count for map '%s' AFTER CloseIdleConnections(): %d", sslCtxByTupleMap, tupleMapCountAfter)

	// We expect that one entry will be removed from each map, if that map was populated to begin with.
	expectedCtxCount := ctxMapCountBefore
	if expectedCtxCount > 0 {
		expectedCtxCount--
	}
	assert.Equal(t, expectedCtxCount, ctxMapCountAfter, "ssl_sock_by_ctx map count after cleanup is not what we expect")

	expectedTupleCount := tupleMapCountBefore
	if expectedTupleCount > 0 {
		expectedTupleCount--
	}
	assert.Equal(t, expectedTupleCount, tupleMapCountAfter, "ssl_ctx_by_tuple map count after cleanup is not what we expect")

	requestsExist := make([]bool, len(requests))
	require.Eventually(t, func() bool {
		stats := getHTTPLikeProtocolStats(t, usmMonitor, protocols.HTTP)
		if stats == nil {
			return false
		}

		if len(stats) == 0 {
			return false
		}

		for reqIndex, req := range requests {
			if !requestsExist[reqIndex] {
				requestsExist[reqIndex] = isRequestIncluded(stats, req)
			}
		}

		for reqIndex, exists := range requestsExist {
			if !exists {
				t.Logf("request %d was not found (req %v)", reqIndex+1, requests[reqIndex])
			}
		}

		return true
	}, 3*time.Second, 100*time.Millisecond, "connection not found")
	if t.Failed() {
		// Dump relevant maps on failure
		ebpftest.DumpMapsTestHelper(t, usmMonitor.DumpMaps, sslSockByCtxMap, sslCtxByTupleMap)
		t.FailNow()
	}
}

// TestPIDKeyedMapNameUniqueness verifies that all PID-keyed TLS map names are unique
// within their first 15 characters to prevent collisions from kernel truncation.
//
// eBPF map names are limited to 15 characters by the kernel (BPF_OBJ_NAME_LEN - 1).
// The leak detection system searches maps by truncated names, so names like
// "hash_map_name_10" and "hash_map_name_11" would collide as both truncate to
// "hash_map_name_1".
//
// This test ensures we catch such collisions at compile/test time rather than
// discovering them in production.
func TestPIDKeyedMapNameUniqueness(t *testing.T) {
	names := GetPIDKeyedTLSMapNames()
	require.NotEmpty(t, names, "No PID-keyed map names found")

	truncated := make(map[string]string)
	for _, name := range names {
		truncName := name
		if len(name) > 15 {
			truncName = name[:15]
		}

		if existing, found := truncated[truncName]; found {
			t.Errorf("Map name collision detected:\n"+
				"  Map 1: %q\n"+
				"  Map 2: %q\n"+
				"  Both truncate to: %q\n"+
				"Map names must be unique within their first 15 characters due to kernel limitation (BPF_OBJ_NAME_LEN - 1).",
				existing, name, truncName)
		}
		truncated[truncName] = name
	}

	// Log all truncated names for reference
	t.Logf("Current PID-keyed TLS map names and their truncated forms:")
	for _, name := range names {
		if len(name) > 15 {
			truncName := name[:15]
			t.Logf("  %q -> %q (truncated)", name, truncName)
		} else {
			t.Logf("  %q (no truncation)", name)
		}
	}
}

// TestFdBySSLBioMapLeak verifies that the fd_by_ssl_bio map is properly cleaned
// up when BIO_new_socket is called without a subsequent SSL_set_bio.
//
// The original bug: uretprobe__BIO_new_socket adds entries to fd_by_ssl_bio,
// but entries were only deleted by uprobe__SSL_set_bio. If SSL_set_bio was
// never called (e.g., error paths where BIO_free is called directly), entries
// would leak.
//
// The fix: uprobe__BIO_free now deletes entries when BIO is freed.
//
// This test verifies the fix by:
// 1. Starting a TLS server
// 2. Running a helper program that calls BIO_new_socket without SSL_set_bio
// 3. Verifying that fd_by_ssl_bio map entries do NOT accumulate (leak is fixed)
func TestFdBySSLBioMapLeak(t *testing.T) {
	cfg := NewUSMEmptyConfig()
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableHTTPMonitoring = true

	utils.SkipIfTLSUnsupported(t, cfg)

	// Start HTTPS server
	const serverAddr = "127.0.0.1:8443"
	serverDone := testutil.HTTPServer(t, serverAddr, testutil.Options{
		EnableTLS: true,
	})
	t.Cleanup(serverDone)

	// Setup USM monitor
	monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
	require.NotNil(t, monitor)

	// Get the fd_by_ssl_bio map
	fdBioMap, mapExists, err := monitor.ebpfProgram.Manager.GetMap(fdBySSLBioMap)
	require.NoError(t, err, "Error getting fd_by_ssl_bio map")
	require.True(t, mapExists, "fd_by_ssl_bio map does not exist")
	require.NotNil(t, fdBioMap, "fd_by_ssl_bio map is nil")

	// Count entries before running leak helper
	countBefore := utils.CountMapEntries(t, fdBioMap)
	t.Logf("fd_by_ssl_bio entries before: %d", countBefore)

	// Run the bio_leak helper via Docker to create stale entries
	const numStaleEntries = 50
	runBioLeakHelperDocker(t, "127.0.0.1", "8443", numStaleEntries)

	// Count entries after - they should NOT have increased (leak is fixed)
	countAfter := utils.CountMapEntries(t, fdBioMap)
	t.Logf("fd_by_ssl_bio entries after: %d (attempted to create %d stale entries)", countAfter, numStaleEntries)

	// Verify no entries leaked (entries should be cleaned up)
	entriesAdded := countAfter - countBefore
	assert.Equal(t, 0, entriesAdded,
		"fd_by_ssl_bio map should not have stale entries; before=%d, after=%d, leaked=%d",
		countBefore, countAfter, entriesAdded)

	if t.Failed() {
		ebpftest.DumpMapsTestHelper(t, monitor.DumpMaps, fdBySSLBioMap)
	}
}

// TestSSLSockByCtxMultiThreadedLeak verifies that ssl_sock_by_ctx keeps no entries for TLS
// connections that were used by several threads of the same process once they are closed.
//
// The original bug: the map key held the full pid_tgid, thread ID included. Every thread that did
// I/O on a connection without an entry for its own thread ID went through the tcp_sendmsg fallback
// and inserted one, but SSL_shutdown and tcp_close only delete the entry of the thread closing the
// connection. In thread pools the other entries leaked until the process exited, so the map could
// fill up and USM stopped seeing new native TLS connections.
//
// The fix: the key holds the process ID (TGID) instead of pid_tgid, so all threads share one
// entry per SSL context.
//
// This test runs a helper with a thread pool sharing TLS connections. While they are still open,
// it checks that the helper's PID has at most one entry per connection; then it closes them all and
// checks (while the helper is still alive) that no entry is left for its PID and that its requests
// were captured and attributed to the right connections.
func TestSSLSockByCtxMultiThreadedLeak(t *testing.T) {
	cfg := NewUSMEmptyConfig()
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableHTTPMonitoring = true

	utils.SkipIfTLSUnsupported(t, cfg)

	const serverHost, serverPort = "127.0.0.1", "8444"
	serverDone := testutil.HTTPServer(t, net.JoinHostPort(serverHost, serverPort), testutil.Options{
		EnableTLS:       true,
		EnableKeepAlive: true,
		ReadTimeout:     time.Minute,
	})
	t.Cleanup(serverDone)

	// Start the helper before the monitor. The container builds the helper first, and the linker
	// maps libssl.so while it runs: if the monitor is already running, it tries to attach to the
	// short-lived linker, fails, and then skips the same library when the helper loads it.
	helper := startSSLThreadPoolHelperDocker(t, serverHost, serverPort)

	monitor := setupUSMTLSMonitor(t, cfg, useExistingConsumer)
	require.NotNil(t, monitor)

	sslSockMap, mapExists, err := monitor.ebpfProgram.Manager.GetMap(sslSockByCtxMap)
	require.NoError(t, err, "Error getting %s map", sslSockByCtxMap)
	require.True(t, mapExists, "%s map does not exist", sslSockByCtxMap)

	pid := helper.pid
	helper.runUntilPaused(t)

	// The helper's connections are still open: there must be at least one entry, and at most one
	// per connection (not one per thread). This also fails if the C and Go key layouts drift, since
	// no entry would match the PID.
	open := countSSLSockByCtxEntriesOfPID(t, sslSockMap, pid)
	t.Logf("%s entries for pid %d with %d connections open: %d", sslSockByCtxMap, pid, sslThreadPoolConnections, open)
	assert.GreaterOrEqual(t, open, 1, "%s has no entry for the open connections", sslSockByCtxMap)
	assert.LessOrEqual(t, open, sslThreadPoolConnections, "%s has more than one entry per open connection", sslSockByCtxMap)

	requests := helper.closeConnections(t)
	require.Positive(t, requests, "the helper did not complete any request")

	leftover := countSSLSockByCtxEntriesOfPID(t, sslSockMap, pid)
	t.Logf("%s entries left for pid %d after closing all connections: %d", sslSockByCtxMap, pid, leftover)
	assert.Zero(t, leftover, "%s has entries for connections that were closed", sslSockByCtxMap)

	// Make sure the connections were actually monitored, and that requests were attributed to the
	// connection they were sent on (the path holds the client port).
	const pathPrefix = "/200/ssl-threadpool/p"
	captured, misattributed := 0, 0
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && captured+misattributed < requests {
		time.Sleep(500 * time.Millisecond)
		for key, stats := range getHTTPLikeProtocolStats(t, monitor, protocols.HTTP) {
			path := key.Path.Content.Get()
			if !strings.HasPrefix(path, pathPrefix) {
				continue
			}
			clientPort, err := strconv.Atoi(path[len(pathPrefix):])
			require.NoError(t, err, "unexpected path %q", path)
			count := 0
			for _, stat := range stats.Data {
				count += stat.Count
			}
			if int(key.SrcPort) == clientPort || int(key.DstPort) == clientPort {
				captured += count
			} else {
				misattributed += count
			}
		}
	}
	t.Logf("requests sent: %d, captured on the right connection: %d, on another connection: %d", requests, captured, misattributed)
	assert.GreaterOrEqual(t, captured, requests*9/10, "less than 90 percent of the helper's requests were captured")
	assert.Zero(t, misattributed, "requests were attributed to the wrong connection")

	if t.Failed() {
		ebpftest.DumpMapsTestHelper(t, monitor.DumpMaps, sslSockByCtxMap, sslCtxByTupleMap)
	}
}

// Verifies that the dead-process cleaner reads the PID from the new ssl_sock_by_ctx key layout:
// entries of a dead PID (and their ssl_ctx_by_tuple entries) are removed, entries of a live PID stay.
func TestSSLSockByCtxDeadPidCleaner(t *testing.T) {
	setNativeTLSPeriodicTerminatedProcessesScanInterval(t, time.Second)
	cfg := NewUSMEmptyConfig()
	cfg.EnableNativeTLSMonitoring = true
	cfg.EnableUSMEventStream = false
	utils.SkipIfTLSUnsupported(t, cfg)
	monitor := setupUSMTLSMonitor(t, cfg, reInitEventConsumer)
	require.NotNil(t, monitor)
	cleanProtocolMaps(t, "ssl", monitor.ebpfProgram.Manager.Manager)

	maps := getMaps(t, monitor.ebpfProgram.Manager.Manager, []string{sslSockByCtxMap, sslCtxByTupleMap})
	sockByCtx, ctxByTuple := maps[0], maps[1]
	require.Equal(t, uint32(unsafe.Sizeof(http.SSLCtxPidTGID{})), sockByCtx.KeySize())

	deadPid := uint32(findNonExistingPid(t))
	// the cleaner never counts its own process as alive, so use a separate process
	cmd, cancel := startDummyProgram(t)
	alivePid := uint32(cmd.Process.Pid)
	var sock http.SslSock

	// A cleaner run takes its snapshot of live PIDs during its /proc walk and deletes at the end, so
	// a run whose walk started before the dummy process existed would also delete its entries. Wait
	// for a run to remove a sentinel inserted after the dummy started: runs don't overlap, so every
	// later run starts its walk after the dummy exists.
	sentinelKey := http.SSLCtxPidTGID{Pid: deadPid, Ctx: 0x9999}
	require.NoError(t, sockByCtx.Put(unsafe.Pointer(&sentinelKey), unsafe.Pointer(&sock)))
	require.Eventually(t, func() bool {
		return !findKeyInMap(sockByCtx, sentinelKey)
	}, 10*time.Second, 100*time.Millisecond, "the cleaner did not remove the sentinel entry")

	deadKey := http.SSLCtxPidTGID{Pid: deadPid, Ctx: 0x1111}
	aliveKey := http.SSLCtxPidTGID{Pid: alivePid, Ctx: 0x2222}
	require.NoError(t, sockByCtx.Put(unsafe.Pointer(&deadKey), unsafe.Pointer(&sock)))
	require.NoError(t, sockByCtx.Put(unsafe.Pointer(&aliveKey), unsafe.Pointer(&sock)))
	deadTup := http.ConnTuple{Sport: 1111, Dport: 443, Pid: deadPid}
	aliveTup := http.ConnTuple{Sport: 2222, Dport: 443, Pid: alivePid}
	deadCtx, aliveCtx := uint64(0x1111), uint64(0x2222)
	require.NoError(t, ctxByTuple.Put(unsafe.Pointer(&deadTup), unsafe.Pointer(&deadCtx)))
	require.NoError(t, ctxByTuple.Put(unsafe.Pointer(&aliveTup), unsafe.Pointer(&aliveCtx)))

	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		assert.False(ct, findKeyInMap(sockByCtx, deadKey), "dead pid entry still in ssl_sock_by_ctx")
		assert.False(ct, findKeyInMap(ctxByTuple, deadTup), "dead pid tuple still in ssl_ctx_by_tuple")
	}, 10*time.Second, 500*time.Millisecond)
	// The dead tuple is removed in the last step of a cleaner run, after that run has checked both
	// ssl_sock_by_ctx entries, so the live entries must still be there.
	assert.True(t, findKeyInMap(sockByCtx, aliveKey), "live pid entry was removed from ssl_sock_by_ctx")
	assert.True(t, findKeyInMap(ctxByTuple, aliveTup), "live pid tuple was removed from ssl_ctx_by_tuple")

	// once that process exits, its entries go too
	cancel()
	_ = cmd.Wait()
	require.EventuallyWithT(t, func(ct *assert.CollectT) {
		assert.False(ct, findKeyInMap(sockByCtx, aliveKey), "exited pid entry still in ssl_sock_by_ctx")
		assert.False(ct, findKeyInMap(ctxByTuple, aliveTup), "exited pid tuple still in ssl_ctx_by_tuple")
	}, 10*time.Second, 500*time.Millisecond)
}

// countSSLSockByCtxEntriesOfPID counts the ssl_sock_by_ctx entries that belong to the given process.
func countSSLSockByCtxEntriesOfPID(t *testing.T, m *ebpf.Map, pid int) int {
	t.Helper()
	var key http.SSLCtxPidTGID
	var value http.SslSock
	count := 0
	iter := m.Iterate()
	for iter.Next(unsafe.Pointer(&key), unsafe.Pointer(&value)) {
		if int(key.Pid) == pid {
			count++
		}
	}
	require.NoError(t, iter.Err())
	return count
}

// sslThreadPoolConnections is the number of TLS connections the ssl_threadpool helper keeps open.
const sslThreadPoolConnections = 20

// sslThreadPoolHelper is a running ssl_threadpool helper, waiting for the start signal. The helper
// stays alive until the test ends.
type sslThreadPoolHelper struct {
	pid    int
	outDir string
}

// readInt reads an integer the helper wrote to its output directory.
func (h sslThreadPoolHelper) readInt(name string) (int, error) {
	data, err := os.ReadFile(filepath.Join(h.outDir, name))
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// runUntilPaused waits for the helper to be traced, lets it send its requests, and returns once
// it has paused with all its connections still open.
func (h sslThreadPoolHelper) runUntilPaused(t *testing.T) {
	t.Helper()
	utils.WaitForProgramsToBeTraced(t, consts.USMModuleName, UsmTLSAttacherName, h.pid, utils.ManualTracingFallbackEnabled)
	require.NoError(t, os.WriteFile(filepath.Join(h.outDir, "start"), nil, 0o644))
	require.Eventually(t, func() bool {
		_, err := h.readInt("paused")
		return err == nil
	}, time.Minute, 200*time.Millisecond, "the helper did not finish its requests")
}

// closeConnections lets a paused helper close all its connections, and returns how many requests
// it completed.
func (h sslThreadPoolHelper) closeConnections(t *testing.T) int {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(h.outDir, "close"), nil, 0o644))
	var requests int
	require.Eventually(t, func() bool {
		var err error
		requests, err = h.readInt("requests")
		return err == nil
	}, time.Minute, 200*time.Millisecond, "the helper did not close its connections")
	return requests
}

// startSSLThreadPoolHelperDocker builds and starts the ssl_threadpool helper in a Docker container.
// The helper writes its PID, then waits for the start signal before opening any connection.
func startSSLThreadPoolHelperDocker(t *testing.T, host, port string) sslThreadPoolHelper {
	t.Helper()

	curDir, err := testutil.CurDir()
	require.NoError(t, err, "Failed to get current directory")
	testDataDir := filepath.Join(curDir, "testdata", "ssl_threadpool_test")
	outDir := t.TempDir()

	env := []string{
		"TESTDIR=" + testDataDir,
		"OUTDIR=" + outDir,
		"HOST=" + host,
		"PORT=" + port,
		"CONNECTIONS=" + strconv.Itoa(sslThreadPoolConnections),
		"THREADS=16",
		"REQUESTS_PER_THREAD=500",
		"ROTATIONS=300",
	}

	scanner, err := globalutils.NewScanner(regexp.MustCompile("SSL_THREADPOOL_READY"), globalutils.NoPattern)
	require.NoError(t, err, "failed to create pattern scanner")

	dockerCfg := dockerutils.NewComposeConfig(
		dockerutils.NewBaseConfig(
			"ssl-threadpool",
			scanner,
			dockerutils.WithEnv(env),
			dockerutils.WithTimeout(2*time.Minute),
			dockerutils.WithRetries(1),
		),
		filepath.Join(testDataDir, "docker-compose.yml"))
	require.NoError(t, dockerutils.Run(t, dockerCfg), "failed to run ssl_threadpool docker container")

	helper := sslThreadPoolHelper{outDir: outDir}
	helper.pid, err = helper.readInt("pid")
	require.NoError(t, err)
	return helper
}

// runBioLeakHelperDocker runs the bio_leak helper in a Docker container.
// The helper calls BIO_new_socket without SSL_set_bio to test map cleanup.
func runBioLeakHelperDocker(t *testing.T, host, port string, numEntries int) {
	t.Helper()

	// Get the testdata directory path using CurDir() which handles
	// the difference between build-time and runtime paths in CI
	curDir, err := testutil.CurDir()
	require.NoError(t, err, "Failed to get current directory")
	testDataDir := filepath.Join(curDir, "testdata", "bio_leak_test")

	env := []string{
		"TESTDIR=" + testDataDir,
		"HOST=" + host,
		"PORT=" + port,
		"NUM_ENTRIES=" + strconv.Itoa(numEntries),
	}

	// Wait for "ready" output which indicates the binary was built and is about to run
	scanner, err := globalutils.NewScanner(regexp.MustCompile("ready"), globalutils.NoPattern)
	require.NoError(t, err, "failed to create pattern scanner")

	dockerCfg := dockerutils.NewComposeConfig(
		dockerutils.NewBaseConfig(
			"bio-leak",
			scanner,
			dockerutils.WithEnv(env),
		),
		filepath.Join(testDataDir, "docker-compose.yml"))

	err = dockerutils.Run(t, dockerCfg)
	require.NoError(t, err, "failed to run bio_leak docker container")
}
