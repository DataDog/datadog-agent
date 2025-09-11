// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Built with linux_bpf since the tests need to run as root for testing the privileged access.
//go:build linux && linux_bpf

package file

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	// System probe imports for fd-transfer
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

type testPrivilegedHandler struct {
	wg      sync.WaitGroup
	handler http.Handler
	called  bool
}

func (h *testPrivilegedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Lock due to the setreuid(2) call below.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Get current EUID to restore later
	originalEUID := syscall.Geteuid()

	// Temporarily escalate to root for file access (this thread only).  Need to
	// use the raw system call since the wrapper sets it on all threads.
	if _, _, err := syscall.Syscall(syscall.SYS_SETREUID, ^uintptr(0), 0, 0); err != 0 {
		http.Error(w, fmt.Sprintf("Privilege escalation failed: %v", err),
			http.StatusInternalServerError)
		return
	}

	// Use a wait group to allow the test cleanup to wait for the handler to
	// finish, since Shutdown on the http server does not wait for hijacked
	// connections (which the privileged logs module uses).
	h.wg.Add(1)

	defer func() {
		if _, _, err := syscall.Syscall(syscall.SYS_SETREUID, ^uintptr(0), uintptr(originalEUID), 0); err != 0 {
			// Log error but can't do much else in test context
			log.Errorf("Failed to restore EUID: %v", err)
		}
		h.wg.Done()
	}()

	// Call the wrapped handler
	h.called = true
	h.handler.ServeHTTP(w, r)
}

type PrivilegedLogsTestSetupStrategy struct {
	tempDirs [2]string
}

func (s *PrivilegedLogsTestSetupStrategy) Setup(t *testing.T) TestSetupResult {
	// The tests are run as root since they're part of the system-probe tests.
	// This allows us to test the privileged logs module by keeping the log
	// files inaccessible to the logs tailing code but accessible to the
	// privileged logs module.  We do this by changing the EUID to an
	// unprivileged user here but changing it back to root in the privileged
	// logs test server, and by creating all log files with 000 permissions
	// which only EUID root can override.
	unprivilegedUID := 0
	sudoUID := os.Getenv("SUDO_UID")
	if sudoUID != "" {
		var err error
		unprivilegedUID, err = strconv.Atoi(sudoUID)
		require.NoError(t, err)
	}

	if unprivilegedUID == 0 {
		user, err := user.Lookup("nobody")
		require.NoError(t, err)
		unprivilegedUID, err = strconv.Atoi(user.Uid)
		require.NoError(t, err)
	}

	err := syscall.Seteuid(unprivilegedUID)
	require.NoError(t, err)
	t.Cleanup(func() {
		err := syscall.Seteuid(0)
		require.NoError(t, err)
	})

	// Set up privileged-logs server
	setupTestServerForLauncher(t)

	// Create temp directories before setting umask
	s.tempDirs = [2]string{}
	for i := 0; i < 2; i++ {
		testDir := t.TempDir()
		s.tempDirs[i] = testDir
	}

	// Set umask so all created files have 000 permissions and are not readable
	// by the unprivileged user
	oldUmask := syscall.Umask(0777)
	t.Cleanup(func() {
		syscall.Umask(oldUmask)
	})

	return TestSetupResult{TestDirs: s.tempDirs[:]}
}

type PrivilegedLogsLauncherTestSuite struct {
	BaseLauncherTestSuite
}

// setupTestServer creates a test server with the privileged logs module
func setupTestServerForLauncher(t *testing.T) {
	// Create the privileged logs module
	cfg := &sysconfigtypes.Config{}
	deps := module.FactoryDependencies{}

	fdModule, err := modules.PrivilegedLogs.Fn(cfg, deps)
	if err != nil {
		t.Fatalf("Failed to create privileged logs module: %v", err)
	}

	// Use /tmp for shorter socket paths to avoid Unix socket limits
	tempDir, err := os.MkdirTemp("/tmp", "fdtest")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	socketPath := filepath.Join(tempDir, "fd.sock")
	listener, err := server.NewListener(socketPath)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create listener: %v", err)
	}

	// Set up HTTP router and register the module
	httpMux := mux.NewRouter()
	router := module.NewRouter("privileged_logs", httpMux)
	err = fdModule.Register(router)
	if err != nil {
		listener.Close()
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to register module: %v", err)
	}

	testHandler := &testPrivilegedHandler{
		handler: httpMux,
	}
	httpServer := &http.Server{
		Handler: testHandler,
	}

	go func() {
		if err := httpServer.Serve(listener); err != nil && err != http.ErrServerClosed {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to be ready
	require.EventuallyWithT(t, func(collect *assert.CollectT) {
		conn, err := net.Dial("unix", socketPath)
		require.NoError(collect, err)
		conn.Close()
	}, 1*time.Second, 10*time.Millisecond)

	t.Cleanup(func() {
		httpServer.Shutdown(context.Background())
		testHandler.wg.Wait()
		listener.Close()
		os.RemoveAll(tempDir)

		// Safety check since if the umask change is mistakenly removed, the
		// test will pass but the fd-transfer will not be used.
		require.True(t, testHandler.called, "fd-transfer was not used")
	})

	systemProbeConfig := configmock.NewSystemProbe(t)
	systemProbeConfig.SetWithoutSource("privileged_logs.enabled", true)
	systemProbeConfig.SetWithoutSource("system_probe_config.sysprobe_socket", socketPath)
}

func (suite *PrivilegedLogsLauncherTestSuite) SetupSuite() {
	suite.setupStrategy = &PrivilegedLogsTestSetupStrategy{}
}

func TestPrivilegedLogsLauncherTestSuite(t *testing.T) {
	suite.Run(t, new(PrivilegedLogsLauncherTestSuite))
}

func TestPrivilegedLogsLauncherTestSuiteWithConfigID(t *testing.T) {
	s := new(PrivilegedLogsLauncherTestSuite)
	s.configID = "123456789"
	suite.Run(t, s)
}

func TestPrivilegedLogsLauncherScanStartNewTailer(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanStartNewTailerTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherWithConcurrentContainerTailer(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherWithConcurrentContainerTailerTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherTailFromTheBeginning(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherTailFromTheBeginningTest(t, setup.tempDirs[:], true)
}

func TestPrivilegedLogsLauncherSetTail(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherSetTailTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherConfigIdentifier(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherConfigIdentifierTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherScanWithTooManyFiles(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanWithTooManyFilesTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherUpdatesSourceForExistingTailer(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherUpdatesSourceForExistingTailerTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherScanRecentFilesWithRemoval(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanRecentFilesWithRemovalTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherScanRecentFilesWithNewFiles(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanRecentFilesWithNewFilesTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherFileRotation(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherFileRotationTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherFileDetectionSingleScan(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherFileDetectionSingleScanTest(t, setup.tempDirs[:])
}

// setupPrivilegedLogsTest is a helper type for privileged logs test setup
type privilegedLogsTestSetup struct {
	tempDirs [2]string
}

// setupPrivilegedLogsTest sets up the privileged logs test environment
func setupPrivilegedLogsTest(t *testing.T) *privilegedLogsTestSetup {
	strategy := &PrivilegedLogsTestSetupStrategy{}
	result := strategy.Setup(t)

	return &privilegedLogsTestSetup{
		tempDirs: [2]string{result.TestDirs[0], result.TestDirs[1]},
	}
}

func TestPrivilegedLogsLauncherScanStartNewTailerForEmptyFile(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanStartNewTailerForEmptyFileTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherScanStartNewTailerWithOneLine(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanStartNewTailerWithOneLineTest(t, setup.tempDirs[:])
}

func TestPrivilegedLogsLauncherScanStartNewTailerWithLongLine(t *testing.T) {
	setup := setupPrivilegedLogsTest(t)
	runLauncherScanStartNewTailerWithLongLineTest(t, setup.tempDirs[:])
}
