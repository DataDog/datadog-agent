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
	"golang.org/x/sys/unix"
	authenticationv1 "k8s.io/api/authentication/v1"

	"github.com/DataDog/datadog-agent/cmd/cws-instrumentation/subcommands/injectcmd"
	"github.com/DataDog/datadog-agent/pkg/security/tests/testutils"
	"github.com/DataDog/datadog-agent/pkg/security/utils/k8sutils"
)

var (
	bpfLoad                 bool
	bpfClone                bool
	capsetProcessCreds      bool
	k8sUserSession          bool
	setupAndRunIMDSTest     bool
	setupIMDSTest           bool
	cleanupIMDSTest         bool
	runIMDSTest             bool
	userSessionExecutable   string
	userSessionOpenPath     string
	syscallDriftTest        bool
	loginUIDTest            bool
	loginUIDPath            string
	loginUIDEventType       string
	loginUIDValue           int
	goSpanTest              bool
	goSpanExecTest          bool
	goSpanNoLabelsTest      bool
	goSpanNoLabelsExecTest  bool
	goSpanForkExecTest      bool
	goSpanSpanID            string
	goSpanLocalRootSpanID   string
	goSpanFilePath          string
	goSpanExecTarget        string
	ddtraceSpanTest         bool
	ddtraceSpanExecTest     bool
	ddtraceNoSpanTest       bool
	ddtraceNoSpanExecTest   bool
	ddtraceSpanForkExecTest bool
	ddtraceSpanFilePath     string
	ddtraceSpanExecTarget   string
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

// setupGoTracerMemfd creates and seals the tracer-info memfd that drives the
// agent's resolveGoLabels flow. Shared by all Go-span test modes (with/without
// pprof labels, open or exec).
func setupGoTracerMemfd(serviceName, memfdName string) (int, error) {
	type TracerMeta struct {
		SchemaVersion  uint8  `msgpack:"schema_version"`
		TracerLanguage string `msgpack:"tracer_language"`
		TracerVersion  string `msgpack:"tracer_version"`
		Hostname       string `msgpack:"hostname"`
		ServiceName    string `msgpack:"service_name"`
	}
	data, err := msgpack.Marshal(&TracerMeta{
		SchemaVersion:  2,
		TracerLanguage: "go",
		TracerVersion:  "0.0.1-test",
		Hostname:       "test",
		ServiceName:    serviceName,
	})
	if err != nil {
		return -1, fmt.Errorf("msgpack marshal: %w", err)
	}

	fd, err := unix.MemfdCreate(memfdName, unix.MFD_ALLOW_SEALING)
	if err != nil {
		return -1, fmt.Errorf("memfd_create: %w", err)
	}
	if _, err := unix.Write(fd, data); err != nil {
		unix.Close(fd)
		return -1, fmt.Errorf("memfd write: %w", err)
	}
	const fAddSeals = 1033 // F_ADD_SEALS
	const fSealWrite = 0x0008
	const fSealShrink = 0x0002
	const fSealGrow = 0x0004
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, uintptr(fd), fAddSeals, fSealWrite|fSealShrink|fSealGrow); errno != 0 {
		unix.Close(fd)
		return -1, fmt.Errorf("memfd seal: %w", errno)
	}

	// Wait for the agent to process the memfd seal event and populate the
	// go_labels_procs BPF map.
	time.Sleep(500 * time.Millisecond)
	return fd, nil
}

// triggerOpen creates filePath, closes, and unlinks — the CWS rule fires on
// the open hook before unlink.
func triggerOpen(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	f.Close()
	os.Remove(filePath)
	return nil
}

// triggerExec execs the target binary with `--reference /etc/passwd <filePath>`
// so the existing exec rule (exec.args_flags == "reference") matches. The
// current process image is replaced; the eBPF probe captures the span context
// at prepare_binprm, before the replacement.
func triggerExec(target, filePath string) error {
	if target == "" {
		return fmt.Errorf("exec target is required")
	}
	argv := []string{target, "--reference", "/etc/passwd", filePath}
	return syscall.Exec(target, argv, os.Environ())
}

// triggerForkExec fork+execs the target binary via os/exec.Cmd.Run — i.e. the
// child runs in a brand-new tgid. This is the canonical "Go program shells out
// to a subprocess" pattern (the one os/exec exposes), and is the scenario in
// which the agent currently loses the parent's APM correlation: the eBPF fork
// hook does not propagate go_labels_procs from the
// parent's tgid to the child's, so fill_span_context in the child's exec hook
// returns an empty span context.
//
// The test that drives this asserts the empty result, pinning the current
// behaviour; when fork-time inheritance is later added, that assertion will
// need to flip.
func triggerForkExec(target, filePath string) error {
	if target == "" {
		return fmt.Errorf("exec target is required")
	}
	cmd := exec.Command(target, "--reference", "/etc/passwd", filePath)
	return cmd.Run()
}

// RunGoSpanTest creates a tracer-info memfd (triggering Go label offset
// resolution), optionally sets pprof labels (skipped for negative-path
// scenarios), and then triggers the syscall the rule watches. The trigger is:
//   - open of filePath when execTarget == ""
//   - in-process execve of execTarget when forkExec == false
//   - fork+execve (os/exec.Cmd.Run) of execTarget when forkExec == true
//
// The fork+execve mode is used by the "fork_exec_no_inheritance" regression
// test, which pins the current behaviour where the child's brand-new tgid has
// no entry in go_labels_procs.
func RunGoSpanTest(spanID, localRootSpanID, filePath, execTarget string, setLabels, forkExec bool) error {
	fd, err := setupGoTracerMemfd("go-span-test", "datadog-tracer-info-gotest01")
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	if setLabels {
		// Set pprof labels exactly like dd-trace-go does.
		// Keys: "span id" and "local root span id", values: decimal strings.
		labels := pprof.Labels("span id", spanID, "local root span id", localRootSpanID)
		ctx := pprof.WithLabels(context.Background(), labels)
		pprof.SetGoroutineLabels(ctx)
		defer pprof.SetGoroutineLabels(context.Background())
	}

	if forkExec {
		return triggerForkExec(execTarget, filePath)
	}
	if execTarget != "" {
		return triggerExec(execTarget, filePath)
	}
	return triggerOpen(filePath)
}

// RunDDTraceSpanTest uses dd-trace-go to create a real span (which sets pprof
// labels via the profiler code-hotspots integration) and then triggers the
// syscall the rule watches. Trigger selection mirrors RunGoSpanTest:
//   - open of filePath when execTarget == ""
//   - in-process execve when forkExec == false
//   - fork+execve when forkExec == true
//
// If startSpan is false, the tracer is started but no active span is created —
// the eBPF reader should yield an empty span context (negative path).
func RunDDTraceSpanTest(filePath, execTarget string, startSpan, forkExec bool) error {
	fd, err := setupGoTracerMemfd("ddtrace-test", "datadog-tracer-info-ddtrace0")
	if err != nil {
		return err
	}
	defer unix.Close(fd)

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

	var span *tracer.Span
	if startSpan {
		// dd-trace-go will automatically set pprof labels "span id" and
		// "local root span id" on the current goroutine.
		var ctx context.Context
		span, ctx = tracer.StartSpanFromContext(context.Background(), "test.operation")
		_ = ctx

		// Print the span ID and local root span ID so the test can parse
		// and verify them.
		spanID := span.Context().SpanID()
		localRootSpanID := span.Root().Context().SpanID()
		fmt.Printf("ddtrace_span_id=%d\n", spanID)
		fmt.Printf("ddtrace_local_root_span_id=%d\n", localRootSpanID)

		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer span.Finish()
	}

	if forkExec {
		return triggerForkExec(execTarget, filePath)
	}
	if execTarget != "" {
		return triggerExec(execTarget, filePath)
	}
	return triggerOpen(filePath)
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
	flag.BoolVar(&goSpanTest, "go-span-test", false, "when set, runs the Go pprof labels span test (open, labels set)")
	flag.BoolVar(&goSpanExecTest, "go-span-exec-test", false, "when set, runs the Go pprof labels span exec test (exec, labels set)")
	flag.BoolVar(&goSpanNoLabelsTest, "go-span-no-labels-test", false, "when set, runs the Go span open test WITHOUT setting pprof labels (negative path)")
	flag.BoolVar(&goSpanNoLabelsExecTest, "go-span-no-labels-exec-test", false, "when set, runs the Go span exec test WITHOUT setting pprof labels (negative path)")
	flag.BoolVar(&goSpanForkExecTest, "go-span-fork-exec-test", false, "when set, sets pprof labels then fork+execs the target via os/exec (parent's labels are not inherited by the child's new tgid — pins the current fork+exec gap)")
	flag.StringVar(&goSpanSpanID, "go-span-span-id", "", "span ID for the Go span test (decimal string)")
	flag.StringVar(&goSpanLocalRootSpanID, "go-span-local-root-span-id", "", "local root span ID for the Go span test (decimal string)")
	flag.StringVar(&goSpanFilePath, "go-span-file-path", "", "file path to open / touch for the Go span test")
	flag.StringVar(&goSpanExecTarget, "go-span-exec-target", "", "executable to exec for the Go span exec test (e.g. /usr/bin/touch)")
	flag.BoolVar(&ddtraceSpanTest, "ddtrace-span-test", false, "when set, runs the dd-trace-go span test (open, active span)")
	flag.BoolVar(&ddtraceSpanExecTest, "ddtrace-span-exec-test", false, "when set, runs the dd-trace-go span exec test (exec, active span)")
	flag.BoolVar(&ddtraceNoSpanTest, "ddtrace-no-span-test", false, "when set, runs the dd-trace-go open test WITHOUT an active span (negative path)")
	flag.BoolVar(&ddtraceNoSpanExecTest, "ddtrace-no-span-exec-test", false, "when set, runs the dd-trace-go exec test WITHOUT an active span (negative path)")
	flag.BoolVar(&ddtraceSpanForkExecTest, "ddtrace-span-fork-exec-test", false, "when set, starts an active dd-trace-go span and fork+execs the target via os/exec (pins the current fork+exec gap)")
	flag.StringVar(&ddtraceSpanFilePath, "ddtrace-span-file-path", "", "file path to open / touch for the dd-trace-go span test")
	flag.StringVar(&ddtraceSpanExecTarget, "ddtrace-span-exec-target", "", "executable to exec for the dd-trace-go span exec test (e.g. /usr/bin/touch)")

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

	switch {
	case goSpanTest:
		if err := RunGoSpanTest(goSpanSpanID, goSpanLocalRootSpanID, goSpanFilePath, "", true, false); err != nil {
			panic(err)
		}
	case goSpanExecTest:
		if err := RunGoSpanTest(goSpanSpanID, goSpanLocalRootSpanID, goSpanFilePath, goSpanExecTarget, true, false); err != nil {
			panic(err)
		}
	case goSpanNoLabelsTest:
		if err := RunGoSpanTest("", "", goSpanFilePath, "", false, false); err != nil {
			panic(err)
		}
	case goSpanNoLabelsExecTest:
		if err := RunGoSpanTest("", "", goSpanFilePath, goSpanExecTarget, false, false); err != nil {
			panic(err)
		}
	case goSpanForkExecTest:
		if err := RunGoSpanTest(goSpanSpanID, goSpanLocalRootSpanID, goSpanFilePath, goSpanExecTarget, true, true); err != nil {
			panic(err)
		}
	}

	switch {
	case ddtraceSpanTest:
		if err := RunDDTraceSpanTest(ddtraceSpanFilePath, "", true, false); err != nil {
			panic(err)
		}
	case ddtraceSpanExecTest:
		if err := RunDDTraceSpanTest(ddtraceSpanFilePath, ddtraceSpanExecTarget, true, false); err != nil {
			panic(err)
		}
	case ddtraceNoSpanTest:
		if err := RunDDTraceSpanTest(ddtraceSpanFilePath, "", false, false); err != nil {
			panic(err)
		}
	case ddtraceNoSpanExecTest:
		if err := RunDDTraceSpanTest(ddtraceSpanFilePath, ddtraceSpanExecTarget, false, false); err != nil {
			panic(err)
		}
	case ddtraceSpanForkExecTest:
		if err := RunDDTraceSpanTest(ddtraceSpanFilePath, ddtraceSpanExecTarget, true, true); err != nil {
			panic(err)
		}
	}
}
