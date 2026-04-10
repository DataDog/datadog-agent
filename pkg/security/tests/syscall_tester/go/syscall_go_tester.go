// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build syscalltesters

// Package main holds main related files
package main

import (
	"bytes"
	"context"
	_ "embed"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"runtime/pprof"
	"strconv"
	"syscall"
	"time"
	"unsafe"

	"github.com/DataDog/dd-trace-go/v2/ddtrace/tracer"
	manager "github.com/DataDog/ebpf-manager"
	"github.com/syndtr/gocapability/capability"
	"github.com/vishvananda/netlink"
	"github.com/vmihailenco/msgpack/v5"
	authenticationv1 "k8s.io/api/authentication/v1"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/subcommands/injectcmd"
	"github.com/DataDog/datadog-agent/pkg/security/tests/testutils"
	"github.com/DataDog/datadog-agent/pkg/security/utils/k8sutils"
)

var (
	bpfLoad               bool
	bpfClone              bool
	capsetProcessCreds    bool
	k8sUserSession        bool
	setupAndRunIMDSTest   bool
	setupIMDSTest         bool
	cleanupIMDSTest       bool
	runIMDSTest           bool
	userSessionExecutable string
	userSessionOpenPath   string
	syscallDriftTest      bool
	loginUIDTest          bool
	loginUIDPath          string
	loginUIDEventType     string
	loginUIDValue         int
	goSpanTest            bool
	goSpanSpanID          string
	goSpanLocalRootSpanID string
	goSpanFilePath        string
	ddtraceSpanTest       bool
	ddtraceSpanFilePath   string
)

//go:embed ebpf_probe.o
var ebpfProbe []byte

func BPFClone(m *manager.Manager) error {
	if _, err := m.CloneMap("cache", "cache_clone", manager.MapOptions{}); err != nil {
		return fmt.Errorf("couldn't clone 'cache' map: %w", err)
	}
	return nil
}

func BPFLoad() error {
	m := &manager.Manager{
		Probes: []*manager.Probe{
			{
				ProbeIdentificationPair: manager.ProbeIdentificationPair{
					UID:          "MyVFSOpen",
					EBPFFuncName: "kprobe_vfs_open",
				},
			},
		},
		Maps: []*manager.Map{
			{
				Name: "cache",
			},
			{
				Name: "is_discarded_by_inode_gen",
			},
		},
	}
	defer func() {
		_ = m.Stop(manager.CleanAll)
	}()

	if err := m.Init(bytes.NewReader(ebpfProbe)); err != nil {
		return fmt.Errorf("failed to initialize manager: %w", err)
	}

	if bpfClone {
		return BPFClone(m)
	}

	return nil
}

func CapsetTest() error {
	threadCapabilities, err := capability.NewPid2(0)
	if err != nil {
		return err
	}
	if err := threadCapabilities.Load(); err != nil {
		return err
	}

	threadCapabilities.Unset(capability.PERMITTED|capability.EFFECTIVE, capability.CAP_SYS_BOOT)
	threadCapabilities.Unset(capability.EFFECTIVE, capability.CAP_WAKE_ALARM)
	return threadCapabilities.Apply(capability.CAPS)
}

func K8SUserSessionTest(executable string, openPath string) error {
	cmd := []string{executable, "--reference", "/etc/passwd"}
	if len(openPath) > 0 {
		cmd = append(cmd, openPath)
	}

	// prepare K8S user session context
	data, err := k8sutils.PrepareK8SUserSessionContext(&authenticationv1.UserInfo{
		Username: "qwerty.azerty@datadoghq.com",
		UID:      "azerty.qwerty@datadoghq.com",
		Groups: []string{
			"ABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABCABC",
			"DEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEFDEF",
		},
		Extra: map[string]authenticationv1.ExtraValue{
			"my_first_extra_values": []string{
				"GHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHIGHI",
				"JKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKLJKL",
			},
			"my_second_extra_values": []string{
				"MNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNOMNO",
				"PQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQRPQR",
				"UVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVWUVW",
				"XYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZXYZ",
			},
		},
	}, 1024)
	if err != nil {
		return err
	}

	if err := injectcmd.InjectUserSessionCmd(
		cmd,
		&injectcmd.InjectCliParams{
			SessionType: "k8s",
			Data:        string(data),
		},
	); err != nil {
		return fmt.Errorf("couldn't run InjectUserSessionCmd: %w", err)
	}

	return nil
}

func SetupAndRunIMDSTest() error {
	// create dummy interface
	dummy, err := SetupIMDSTest()
	defer func() {
		if err = CleanupIMDSTest(dummy); err != nil {
			panic(err)
		}
	}()

	return RunIMDSTest()
}

func RunIMDSTest() error {
	// create fake IMDS server
	imdsServerAddr := testutils.IMDSTestServerIP + ":" + strconv.Itoa(testutils.IMDSTestServerPort)
	imdsServer := testutils.CreateIMDSServer(imdsServerAddr)
	defer func() {
		if err := testutils.StopIMDSserver(imdsServer); err != nil {
			panic(err)
		}
	}()

	// give some time for the server to start
	time.Sleep(5 * time.Second)

	// make IMDS request
	response, err := http.Get(fmt.Sprintf("http://%s%s", imdsServerAddr, testutils.IMDSSecurityCredentialsURL))
	if err != nil {
		return fmt.Errorf("failed to query IMDS server: %v", err)
	}
	return response.Body.Close()
}

func SetupIMDSTest() (*netlink.Dummy, error) {
	// create dummy interface
	return testutils.CreateDummyInterface(testutils.CSMDummyInterface, testutils.IMDSTestServerCIDR)
}

func CleanupIMDSTest(dummy *netlink.Dummy) error {
	return testutils.RemoveDummyInterface(dummy)
}

func RunSyscallDriftTest() error {
	// wait for the syscall monitor period to expire
	time.Sleep(4 * time.Second)

	f, err := os.CreateTemp("/tmp", "syscall-drift-test")
	if err != nil {
		return err
	}
	if _, err = f.Write([]byte("Generating drift syscalls ...")); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}

	tmpFilePtr, err := syscall.BytePtrFromString(f.Name())
	if _, _, err := syscall.Syscall(syscall.SYS_UNLINKAT, 0, uintptr(unsafe.Pointer(tmpFilePtr)), 0); err != 0 {
		return error(err)
	}

	return nil
}

func setSelfLoginUID(uid int) error {
	f, err := os.OpenFile("/proc/self/loginuid", os.O_RDWR, 0755)
	if err != nil {
		return fmt.Errorf("couldn't set login_uid: %v", err)
	}

	if _, err = f.Write([]byte(fmt.Sprintf("%d", uid))); err != nil {
		return fmt.Errorf("couldn't write to login_uid: %v", err)
	}

	if err = f.Close(); err != nil {
		return fmt.Errorf("couldn't close login_uid: %v", err)
	}
	return nil
}

func RunLoginUIDTest() error {
	if loginUIDValue != -1 {
		if err := setSelfLoginUID(loginUIDValue); err != nil {
			return err
		}
	}

	switch loginUIDEventType {
	case "open":
		// open test file to trigger an event
		f, err := os.OpenFile(loginUIDPath, os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			return fmt.Errorf("couldn't create test-auid file: %v", err)
		}
		defer os.Remove(loginUIDPath)

		if err = f.Close(); err != nil {
			return fmt.Errorf("couldn't close test file: %v", err)
		}
	case "exec":
		cmd := exec.Command(loginUIDPath)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("'%s' execution returned an error: %v", loginUIDPath, err)
		}
	case "unlink":
		f, err := os.OpenFile(loginUIDPath, os.O_RDWR|os.O_CREATE, 0755)
		if err != nil {
			return fmt.Errorf("couldn't create test-auid file: %v", err)
		}
		f.Close()
		os.Remove(loginUIDPath)
	default:
		panic("unknown event type")
	}
	return nil
}

// RunGoSpanTest creates a tracer-info memfd (triggering Go label offset resolution),
// sets pprof labels simulating what dd-trace-go does, then opens a file.
// The eBPF reader should extract the span context from the goroutine's pprof labels.
func RunGoSpanTest(spanID, localRootSpanID, filePath string) error {
	// Create and seal a tracer-info memfd with tracer_language="go".
	// This triggers the agent's AddTracerMetadata → resolveGoLabels flow.
	type TracerMeta struct {
		SchemaVersion  uint8  `msgpack:"schema_version"`
		TracerLanguage string `msgpack:"tracer_language"`
		TracerVersion  string `msgpack:"tracer_version"`
		Hostname       string `msgpack:"hostname"`
		ServiceName    string `msgpack:"service_name"`
	}
	meta := TracerMeta{
		SchemaVersion:  2,
		TracerLanguage: "go",
		TracerVersion:  "0.0.1-test",
		Hostname:       "test",
		ServiceName:    "go-span-test",
	}
	data, err := msgpack.Marshal(&meta)
	if err != nil {
		return fmt.Errorf("msgpack marshal: %w", err)
	}

	fd, err := unix.MemfdCreate("datadog-tracer-info-gotest01", unix.MFD_ALLOW_SEALING)
	if err != nil {
		return fmt.Errorf("memfd_create: %w", err)
	}
	defer unix.Close(fd)

	if _, err := unix.Write(fd, data); err != nil {
		return fmt.Errorf("memfd write: %w", err)
	}
	const fAddSeals = 1033 // F_ADD_SEALS
	const fSealWrite = 0x0008
	const fSealShrink = 0x0002
	const fSealGrow = 0x0004
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), fAddSeals, fSealWrite|fSealShrink|fSealGrow); errno != 0 {
		return fmt.Errorf("memfd seal: %w", errno)
	}

	// Wait for the agent to process the memfd seal event and populate the go_labels_procs BPF map.
	time.Sleep(500 * time.Millisecond)

	// Set pprof labels exactly like dd-trace-go does.
	// Keys: "span id" and "local root span id", values: decimal strings.
	labels := pprof.Labels("span id", spanID, "local root span id", localRootSpanID)
	ctx := pprof.WithLabels(context.Background(), labels)
	pprof.SetGoroutineLabels(ctx)
	defer pprof.SetGoroutineLabels(context.Background())

	// Trigger the file open that the CWS rule is watching.
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	f.Close()
	os.Remove(filePath)

	return nil
}

// RunDDTraceSpanTest uses dd-trace-go to create a real span, which sets pprof
// labels automatically via the profiler code hotspots integration. This tests
// the full dd-trace-go → pprof labels → eBPF Go labels reader pipeline.
func RunDDTraceSpanTest(filePath string) error {
	// Create and seal a tracer-info memfd with tracer_language="go".
	type TracerMeta struct {
		SchemaVersion  uint8  `msgpack:"schema_version"`
		TracerLanguage string `msgpack:"tracer_language"`
		TracerVersion  string `msgpack:"tracer_version"`
		Hostname       string `msgpack:"hostname"`
		ServiceName    string `msgpack:"service_name"`
	}
	meta := TracerMeta{
		SchemaVersion:  2,
		TracerLanguage: "go",
		TracerVersion:  "0.0.1-test",
		Hostname:       "test",
		ServiceName:    "ddtrace-test",
	}
	data, err := msgpack.Marshal(&meta)
	if err != nil {
		return fmt.Errorf("msgpack marshal: %w", err)
	}

	fd, err := unix.MemfdCreate("datadog-tracer-info-ddtrace0", unix.MFD_ALLOW_SEALING)
	if err != nil {
		return fmt.Errorf("memfd_create: %w", err)
	}
	defer unix.Close(fd)

	if _, err := unix.Write(fd, data); err != nil {
		return fmt.Errorf("memfd write: %w", err)
	}
	const fAddSeals = 1033
	const fSealWrite = 0x0008
	const fSealShrink = 0x0002
	const fSealGrow = 0x0004
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), fAddSeals, fSealWrite|fSealShrink|fSealGrow); errno != 0 {
		return fmt.Errorf("memfd seal: %w", errno)
	}

	// Wait for the agent to process the memfd seal event and populate the BPF map.
	time.Sleep(500 * time.Millisecond)

	// Start dd-trace-go with:
	// - WithTestDefaults: uses a dummy transport (no real agent needed)
	// - WithProfilerCodeHotspots: enables "span id" and "local root span id" pprof labels
	// - WithService: set a service name
	tracer.Start(
		tracer.WithTestDefaults(nil),
		tracer.WithProfilerCodeHotspots(true),
		tracer.WithService("ddtrace-test"),
		tracer.WithLogStartup(false),
	)
	defer tracer.Stop()

	// Create a span. dd-trace-go will automatically set pprof labels
	// "span id" and "local root span id" on the current goroutine.
	span, ctx := tracer.StartSpanFromContext(context.Background(), "test.operation")

	// Print the span ID and local root span ID so the test can parse and verify them.
	spanID := span.Context().SpanID()
	localRootSpanID := span.Root().Context().SpanID()
	fmt.Printf("ddtrace_span_id=%d\n", spanID)
	fmt.Printf("ddtrace_local_root_span_id=%d\n", localRootSpanID)

	_ = ctx
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Trigger the file open that the CWS rule is watching.
	f, err := os.Create(filePath)
	if err != nil {
		span.Finish()
		return fmt.Errorf("create file: %w", err)
	}
	f.Close()
	os.Remove(filePath)

	span.Finish()
	return nil
}

func main() {
	flag.BoolVar(&bpfLoad, "load-bpf", false, "load the eBPF programs")
	flag.BoolVar(&bpfClone, "clone-bpf", false, "clone maps")
	flag.BoolVar(&capsetProcessCreds, "process-credentials-capset", false, "capset test content")
	flag.BoolVar(&k8sUserSession, "k8s-user-session", false, "user session test")
	flag.StringVar(&userSessionExecutable, "user-session-executable", "", "executable used for the user session test")
	flag.StringVar(&userSessionOpenPath, "user-session-open-path", "", "file used for the user session test")
	flag.BoolVar(&setupAndRunIMDSTest, "setup-and-run-imds-test", false, "when set, runs the IMDS test by creating a dummy interface, binding a fake IMDS server to it and sending an IMDS request")
	flag.BoolVar(&setupIMDSTest, "setup-imds-test", false, "when set, creates a dummy interface and attach the IMDS IP to it")
	flag.BoolVar(&cleanupIMDSTest, "cleanup-imds-test", false, "when set, removes the dummy interface of the IMDS test")
	flag.BoolVar(&runIMDSTest, "run-imds-test", false, "when set, binds an IMDS server locally and sends a query to it")
	flag.BoolVar(&syscallDriftTest, "syscall-drift-test", false, "when set, runs the syscall drift test")
	flag.BoolVar(&loginUIDTest, "login-uid-test", false, "when set, runs the login_uid open test")
	flag.StringVar(&loginUIDPath, "login-uid-path", "", "file used for the login_uid open test")
	flag.StringVar(&loginUIDEventType, "login-uid-event-type", "", "event type used for the login_uid open test")
	flag.IntVar(&loginUIDValue, "login-uid-value", 0, "uid used for the login_uid open test")
	flag.BoolVar(&goSpanTest, "go-span-test", false, "when set, runs the Go pprof labels span test")
	flag.StringVar(&goSpanSpanID, "go-span-span-id", "", "span ID for the Go span test (decimal string)")
	flag.StringVar(&goSpanLocalRootSpanID, "go-span-local-root-span-id", "", "local root span ID for the Go span test (decimal string)")
	flag.StringVar(&goSpanFilePath, "go-span-file-path", "", "file path to open for the Go span test")
	flag.BoolVar(&ddtraceSpanTest, "ddtrace-span-test", false, "when set, runs the dd-trace-go span test")
	flag.StringVar(&ddtraceSpanFilePath, "ddtrace-span-file-path", "", "file path to open for the dd-trace-go span test")

	flag.Parse()

	if bpfLoad {
		if err := BPFLoad(); err != nil {
			panic(err)
		}
	}

	if capsetProcessCreds {
		if err := CapsetTest(); err != nil {
			panic(err)
		}
	}

	if k8sUserSession {
		if err := K8SUserSessionTest(userSessionExecutable, userSessionOpenPath); err != nil {
			panic(err)
		}
	}

	if setupAndRunIMDSTest {
		if err := SetupAndRunIMDSTest(); err != nil {
			panic(err)
		}
	}

	if setupIMDSTest {
		if _, err := SetupIMDSTest(); err != nil {
			panic(err)
		}
	}

	if cleanupIMDSTest {
		if err := CleanupIMDSTest(&netlink.Dummy{
			LinkAttrs: netlink.LinkAttrs{
				Name: testutils.CSMDummyInterface,
			},
		}); err != nil {
			panic(err)
		}
	}

	if runIMDSTest {
		if err := RunIMDSTest(); err != nil {
			panic(err)
		}
	}

	if syscallDriftTest {
		if err := RunSyscallDriftTest(); err != nil {
			panic(err)
		}
	}

	if loginUIDTest {
		if err := RunLoginUIDTest(); err != nil {
			panic(err)
		}
	}

	if goSpanTest {
		if err := RunGoSpanTest(goSpanSpanID, goSpanLocalRootSpanID, goSpanFilePath); err != nil {
			panic(err)
		}
	}

	if ddtraceSpanTest {
		if err := RunDDTraceSpanTest(ddtraceSpanFilePath); err != nil {
			panic(err)
		}
	}
}
