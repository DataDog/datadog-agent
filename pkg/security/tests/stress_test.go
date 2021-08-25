// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build stresstests

package tests

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"syscall"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/probe"
	sprobe "github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

var (
	keepProfile bool
	reportFile  string
	diffBase    string
	duration    int
)

// Stress test of open syscalls
func stressOpen(t *testing.T, rule *rules.RuleDefinition, pathname string, size int) {
	var ruleDefs []*rules.RuleDefinition
	if rule != nil {
		ruleDefs = append(ruleDefs, rule)
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFolder, _, err := test.Path(path.Dir(pathname))
	if err != nil {
		t.Fatal(err)
	}

	os.MkdirAll(testFolder, os.ModePerm)

	testFile, _, err := test.Path(pathname)
	if err != nil {
		t.Fatal(err)
	}

	perfBufferMonitor := test.probe.GetMonitor().GetPerfBufferMonitor()
	perfBufferMonitor.GetAndResetLostCount("events", -1)
	perfBufferMonitor.GetAndResetKernelLostCount("events", -1)

	fnc := func() error {
		f, err := os.Create(testFile)
		if err != nil {
			return err
		}

		if size > 0 {
			data := make([]byte, size, size)
			if n, err := f.Write(data); err != nil || n != 1024 {
				return err
			}
		}

		if err := f.Close(); err != nil {
			return err
		}

		return nil
	}

	opts := StressOpts{
		Duration:    time.Duration(duration) * time.Second,
		KeepProfile: keepProfile,
		DiffBase:    diffBase,
		TopFrom:     "probe",
		ReportFile:  reportFile,
	}

	events := 0
	test.RegisterRuleEventHandler(func(_ *sprobe.Event, _ *rules.Rule) {
		events++
	})
	defer test.RegisterRuleEventHandler(nil)

	report, err := StressIt(t, nil, nil, fnc, opts)
	test.RegisterRuleEventHandler(nil)

	if err != nil {
		t.Fatal(err)
	}

	report.AddMetric("lost", float64(perfBufferMonitor.GetLostCount("events", -1)), "lost")
	report.AddMetric("kernel_lost", float64(perfBufferMonitor.GetAndResetKernelLostCount("events", -1)), "lost")
	report.AddMetric("events", float64(events), "events")
	report.AddMetric("events/sec", float64(events)/report.Duration.Seconds(), "event/s")

	report.Print()

	if report.Delta() < -2.0 {
		t.Error("unexpected performance degradation")

		cmdOutput, _ := exec.Command("pstree").Output()
		fmt.Println(string(cmdOutput))

		cmdOutput, _ = exec.Command("ps", "aux").Output()
		fmt.Println(string(cmdOutput))
	}
}

// goal: measure host abality to handle open syscall without any kprobe, act as a reference
// this benchmark generate syscall but without having kprobe installed

func TestStress_E2EOpenNoKprobe(t *testing.T) {
	stressOpen(t, nil, "folder1/folder2/folder1/folder2/test", 0)
}

// goal: measure the impact of an event catched and passed from the kernel to the userspace
// this benchmark generate event that passs from the kernel to the userspace
func TestStress_E2EOpenEvent(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/folder1/folder2/test" && open.flags & O_CREAT != 0`,
	}

	stressOpen(t, rule, "folder1/folder2/test", 0)
}

// goal: measure the impact on the kprobe only
// this benchmark generate syscall but without having event generated
func TestStress_E2EOpenNoEvent(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/folder1/folder2/test-no-event" && open.flags & O_APPEND != 0`,
	}

	stressOpen(t, rule, "folder1/folder2/test", 0)
}

// goal: measure the impact of an event catched and passed from the kernel to the userspace
// this benchmark generate event that passs from the kernel to the userspace
func TestStress_E2EOpenWrite1KEvent(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/folder1/folder2/test" && open.flags & O_CREAT != 0`,
	}

	stressOpen(t, rule, "folder1/folder2/test", 1024)
}

// goal: measure host abality to handle open syscall without any kprobe, act as a reference
// this benchmark generate syscall but without having kprobe installed

func TestStress_E2EOpenWrite1KNoKprobe(t *testing.T) {
	stressOpen(t, nil, "folder1/folder2/test", 1024)
}

// goal: measure the impact on the kprobe only
// this benchmark generate syscall but without having event generated
func TestStress_E2EOpenWrite1KNoEvent(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/folder1/folder2/test-no-event" && open.flags & O_APPEND != 0`,
	}

	stressOpen(t, rule, "folder1/folder2/test", 1024)
}

// Stress test of fork/exec syscalls
func stressExec(t *testing.T, rule *rules.RuleDefinition, pathname string, executable string) {
	var ruleDefs []*rules.RuleDefinition
	if rule != nil {
		ruleDefs = append(ruleDefs, rule)
	}

	test, err := newTestModule(t, nil, ruleDefs, testOpts{})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFolder, _, err := test.Path(path.Dir(pathname))
	if err != nil {
		t.Fatal(err)
	}

	os.MkdirAll(testFolder, os.ModePerm)

	testFile, _, err := test.Path(pathname)
	if err != nil {
		t.Fatal(err)
	}

	perfBufferMonitor := test.probe.GetMonitor().GetPerfBufferMonitor()
	perfBufferMonitor.GetAndResetLostCount("events", -1)
	perfBufferMonitor.GetAndResetKernelLostCount("events", -1)

	fnc := func() error {
		cmd := exec.Command(executable, testFile)
		if _, err := cmd.CombinedOutput(); err != nil {
			return err
		}

		return nil
	}

	opts := StressOpts{
		Duration:    40 * time.Second,
		KeepProfile: keepProfile,
		DiffBase:    diffBase,
		TopFrom:     "probe",
		ReportFile:  reportFile,
	}

	events := 0
	test.RegisterRuleEventHandler(func(_ *sprobe.Event, _ *rules.Rule) {
		events++
	})
	defer test.RegisterRuleEventHandler(nil)

	report, err := StressIt(t, nil, nil, fnc, opts)
	test.RegisterRuleEventHandler(nil)

	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)

	report.AddMetric("lost", float64(perfBufferMonitor.GetLostCount("events", -1)), "lost")
	report.AddMetric("kernel_lost", float64(perfBufferMonitor.GetAndResetKernelLostCount("events", -1)), "lost")
	report.AddMetric("events", float64(events), "events")
	report.AddMetric("events/sec", float64(events)/report.Duration.Seconds(), "event/s")

	report.Print()

	if report.Delta() < -2.0 {
		t.Error("unexpected performance degradation")

		cmdOutput, _ := exec.Command("pstree").Output()
		fmt.Println(string(cmdOutput))

		cmdOutput, _ = exec.Command("ps", "aux").Output()
		fmt.Println(string(cmdOutput))
	}
}

// goal: measure host abality to handle open syscall without any kprobe, act as a reference
// this benchmark generate syscall but without having kprobe installed

func TestStress_E2EOExecNoKprobe(t *testing.T) {
	executable := "/usr/bin/touch"
	if resolved, err := os.Readlink(executable); err == nil {
		executable = resolved
	} else {
		if os.IsNotExist(err) {
			executable = "/bin/touch"
		}
	}

	stressExec(t, nil, "folder1/folder2/folder1/folder2/test", executable)
}

// goal: measure the impact of an event catched and passed from the kernel to the userspace
// this benchmark generate event that passs from the kernel to the userspace
func TestStress_E2EExecEvent(t *testing.T) {
	executable := "/usr/bin/touch"
	if resolved, err := os.Readlink(executable); err == nil {
		executable = resolved
	} else {
		if os.IsNotExist(err) {
			executable = "/bin/touch"
		}
	}

	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: fmt.Sprintf(`open.file.path == "{{.Root}}/folder1/folder2/test-ancestors" && process.file.name == "%s"`, "touch"),
	}

	stressExec(t, rule, "folder1/folder2/test-ancestors", executable)
}

func BenchmarkERPCDentryResolutionSegment(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/aa/bb/cc/dd/ee" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(b, nil, []*rules.RuleDefinition{rule}, testOpts{disableMapDentryResolution: true})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("aa/bb/cc/dd/ee")
	if err != nil {
		b.Fatal(err)
	}
	_ = os.MkdirAll(path.Dir(testFile), 0755)

	defer os.Remove(testFile)

	var (
		mountID uint32
		inode   uint64
		pathID  uint32
	)
	err = test.GetSignal(b, func() error {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0755)
		if err != nil {
			b.Fatal(err)
		}
		return syscall.Close(int(fd))
	}, func(event *sprobe.Event, _ *rules.Rule) {
		mountID = event.Open.File.MountID
		inode = event.Open.File.Inode
		pathID = event.Open.File.PathID
	})
	if err != nil {
		b.Fatal(err)
	}

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := probe.NewDentryResolver(test.probe)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(test.probe); err != nil {
		b.Fatal(err)
	}
	name, err := resolver.GetNameFromERPC(mountID, inode, pathID)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(name)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		name, err = resolver.GetNameFromERPC(mountID, inode, pathID)
		if err != nil {
			b.Fatal(err)
		}
		if len(name) == 0 || len(name) > 0 && name[0] == 0 {
			b.Log("couldn't resolve segment")
		}
	}

	test.Close()
}

func BenchmarkERPCDentryResolutionPath(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/aa/bb/cc/dd/ee" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(b, nil, []*rules.RuleDefinition{rule}, testOpts{disableMapDentryResolution: true})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("aa/bb/cc/dd/ee")
	if err != nil {
		b.Fatal(err)
	}
	_ = os.MkdirAll(path.Dir(testFile), 0755)

	defer os.Remove(testFile)

	var (
		mountID uint32
		inode   uint64
		pathID  uint32
	)
	err = test.GetSignal(b, func() error {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0755)
		if err != nil {
			b.Fatal(err)
		}
		return syscall.Close(int(fd))
	}, func(event *sprobe.Event, _ *rules.Rule) {
		mountID = event.Open.File.MountID
		inode = event.Open.File.Inode
		pathID = event.Open.File.PathID
	})

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := probe.NewDentryResolver(test.probe)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(test.probe); err != nil {
		b.Fatal(err)
	}
	f, err := resolver.ResolveFromERPC(mountID, inode, pathID)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(f)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		f, err := resolver.ResolveFromERPC(mountID, inode, pathID)
		if err != nil {
			b.Fatal(err)
		}
		if len(f) == 0 || len(f) > 0 && f[0] == 0 {
			b.Log("couldn't resolve path")
		}
	}

	test.Close()
}

func BenchmarkMapDentryResolutionSegment(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/aa/bb/cc/dd/ee" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(b, nil, []*rules.RuleDefinition{rule}, testOpts{disableERPCDentryResolution: true})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("aa/bb/cc/dd/ee")
	if err != nil {
		b.Fatal(err)
	}
	_ = os.MkdirAll(path.Dir(testFile), 0755)

	defer os.Remove(testFile)

	var (
		mountID uint32
		inode   uint64
		pathID  uint32
	)
	err = test.GetSignal(b, func() error {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0755)
		if err != nil {
			b.Fatal(err)
		}
		return syscall.Close(int(fd))
	}, func(event *sprobe.Event, _ *rules.Rule) {
		mountID = event.Open.File.MountID
		inode = event.Open.File.Inode
		pathID = event.Open.File.PathID
	})

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := probe.NewDentryResolver(test.probe)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(test.probe); err != nil {
		b.Fatal(err)
	}
	name, err := resolver.GetNameFromMap(mountID, inode, pathID)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(name)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		name, err = resolver.GetNameFromMap(mountID, inode, pathID)
		if err != nil {
			b.Fatal(err)
		}
		if len(name) == 0 || len(name) > 0 && name[0] == 0 {
			b.Fatal("couldn't resolve segment")
		}
	}

	test.Close()
}

func BenchmarkMapDentryResolutionPath(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.file.path == "{{.Root}}/aa/bb/cc/dd/ee" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(b, nil, []*rules.RuleDefinition{rule}, testOpts{disableERPCDentryResolution: true})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("aa/bb/cc/dd/ee")
	if err != nil {
		b.Fatal(err)
	}
	_ = os.MkdirAll(path.Dir(testFile), 0755)

	defer os.Remove(testFile)

	var (
		mountID uint32
		inode   uint64
		pathID  uint32
	)
	err = test.GetSignal(b, func() error {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0755)
		if err != nil {
			b.Fatal(err)
		}
		return syscall.Close(int(fd))
	}, func(event *sprobe.Event, _ *rules.Rule) {
		mountID = event.Open.File.MountID
		inode = event.Open.File.Inode
		pathID = event.Open.File.PathID
	})

	// create a new dentry resolver to avoid concurrent map access errors
	resolver, err := probe.NewDentryResolver(test.probe)
	if err != nil {
		b.Fatal(err)
	}

	if err := resolver.Start(test.probe); err != nil {
		b.Fatal(err)
	}
	f, err := resolver.ResolveFromMap(mountID, inode, pathID)
	if err != nil {
		b.Fatal(err)
	}
	b.Log(f)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		f, err := resolver.ResolveFromMap(mountID, inode, pathID)
		if err != nil {
			b.Fatal(err)
		}
		if f[0] == 0 {
			b.Fatal("couldn't resolve file")
		}
	}

	test.Close()
}

func init() {
	flag.BoolVar(&keepProfile, "keep-profile", false, "do not delete profile after run")
	flag.StringVar(&reportFile, "report-file", "", "save report of the stress test")
	flag.StringVar(&diffBase, "diff-base", "", "source of base stress report for comparison")
	flag.IntVar(&duration, "duration", 30, "duration of the run in second")
}
