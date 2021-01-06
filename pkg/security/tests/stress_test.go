// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build stresstests

package tests

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"testing"
	"time"

	"github.com/cihub/seelog"

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
	var rules []*rules.RuleDefinition
	if rule != nil {
		rules = append(rules, rule)
	}

	test, err := newTestModule(nil, rules, testOpts{})
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

	eventsStats := test.probe.GetEventsStats()
	eventsStats.GetAndResetLost()

	events := 0
	go func() {
		for range test.events {
			events++
		}
	}()

	var prevLogLevel seelog.LogLevel

	pre := func() (err error) {
		prevLogLevel, err = test.SwapLogLevel(seelog.ErrorLvl)
		return err
	}

	post := func() error {
		_, err := test.SwapLogLevel(prevLogLevel)
		return err
	}

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
		Duration:    time.Duration(30) * time.Second,
		KeepProfile: keepProfile,
		DiffBase:    diffBase,
		TopFrom:     "module",
		ReportFile:  reportFile,
	}

	report, err := StressIt(t, pre, post, fnc, opts)
	if err != nil {
		t.Fatal(err)
	}

	report.AddMetric("lost", float64(eventsStats.GetLost()), "lost")
	report.AddMetric("events", float64(events), "events")
	report.AddMetric("events/sec", float64(events)/report.Duration.Seconds(), "event/s")

	report.Print()

	if report.Delta() < -0.10 {
		t.Error("unexpected performance degradation")
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
		Expression: `open.filename == "{{.Root}}/folder1/folder2/test" && open.flags & O_CREAT != 0`,
	}

	stressOpen(t, rule, "folder1/folder2/test", 0)
}

// goal: measure the impact on the kprobe only
// this benchmark generate syscall but without having event generated
func TestStress_E2EOpenNoEvent(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/folder1/folder2/test-no-event" && open.flags & O_APPEND != 0`,
	}

	stressOpen(t, rule, "folder1/folder2/test", 0)
}

// goal: measure the impact of an event catched and passed from the kernel to the userspace
// this benchmark generate event that passs from the kernel to the userspace
func TestStress_E2EOpenWrite1KEvent(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/folder1/folder2/test" && open.flags & O_CREAT != 0`,
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
		Expression: `open.filename == "{{.Root}}/folder1/folder2/test-no-event" && open.flags & O_APPEND != 0`,
	}

	stressOpen(t, rule, "folder1/folder2/test", 1024)
}

// Stress test of fork/exec syscalls
func stressExec(t *testing.T, rule *rules.RuleDefinition, pathname string, executable string) {
	var rules []*rules.RuleDefinition
	if rule != nil {
		rules = append(rules, rule)
	}

	test, err := newTestModule(nil, rules, testOpts{})
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

	eventsStats := test.probe.GetEventsStats()
	eventsStats.GetAndResetLost()

	events := 0
	go func() {
		for event := range test.events {
			fmt.Printf("<PID: %d\n", event.Event.(*sprobe.Event).Process.Pid)
			events++
		}
	}()

	var prevLogLevel seelog.LogLevel

	pre := func() (err error) {
		prevLogLevel, err = test.SwapLogLevel(seelog.ErrorLvl)
		return err
	}

	post := func() error {
		_, err := test.SwapLogLevel(prevLogLevel)
		return err
	}

	fnc := func() error {
		cmd := exec.Command(executable, testFile)
		if _, err := cmd.CombinedOutput(); err != nil {
			return err
		}
		fmt.Printf(">PID: %d\n", cmd.Process.Pid)

		return nil
	}

	opts := StressOpts{
		Duration:    40 * time.Second,
		KeepProfile: keepProfile,
		DiffBase:    diffBase,
		TopFrom:     "module",
		ReportFile:  reportFile,
	}

	report, err := StressIt(t, pre, post, fnc, opts)
	if err != nil {
		t.Fatal(err)
	}

	time.Sleep(2 * time.Second)

	report.AddMetric("lost", float64(eventsStats.GetLost()), "lost")
	report.AddMetric("events", float64(events), "events")
	report.AddMetric("events/sec", float64(events)/report.Duration.Seconds(), "event/s")

	report.Print()

	if report.Delta() < -0.10 {
		t.Error("unexpected performance degradation")
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
		Expression: fmt.Sprintf(`open.filename == "{{.Root}}/folder1/folder2/test-ancestors" && process.name == "%s"`, "touch"),
	}

	stressExec(t, rule, "folder1/folder2/test-ancestors", executable)
}

func init() {
	flag.BoolVar(&keepProfile, "keep-profile", false, "do not delete profile after run")
	flag.StringVar(&reportFile, "report-file", "", "save report of the stress test")
	flag.StringVar(&diffBase, "diff-base", "", "source of base stress report for comparison")
	flag.IntVar(&duration, "duration", 30, "duration of the run in second")
}
