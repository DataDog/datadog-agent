// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build functionaltests

package tests

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"syscall"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/security/rules"
)

func TestOpen(t *testing.T) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/test-open" && open.flags & O_CREAT != 0`,
	}

	test, err := newTestModule(nil, []*rules.RuleDefinition{rule}, testOpts{enableFilters: true})
	if err != nil {
		t.Fatal(err)
	}
	defer test.Close()

	testFile, testFilePtr, err := test.Path("test-open")
	if err != nil {
		t.Fatal(err)
	}

	fd, _, errno := syscall.Syscall(syscall.SYS_OPENAT, 0, uintptr(testFilePtr), syscall.O_CREAT)
	if errno != 0 {
		t.Fatal(error(errno))
	}
	defer syscall.Close(int(fd))
	defer os.Remove(testFile)

	event, _, err := test.GetEvent()
	if err != nil {
		t.Error(err)
	} else {
		if event.GetType() != "open" {
			t.Errorf("expected open event, got %s", event.GetType())
		}

		if flags := event.Open.Flags; flags != syscall.O_CREAT {
			t.Errorf("expected open mode O_CREAT, got %d", flags)
		}
	}
}

func benchmarkOpenSameFile(b *testing.B, enableFilters bool, rules ...*rules.RuleDefinition) {
	test, err := newTestModule(nil, rules, testOpts{enableFilters: enableFilters})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	testFile, _, err := test.Path("benchtest")
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fd, err := syscall.Open(testFile, syscall.O_CREAT, 0777)
		if err != nil {
			b.Fatal(err)
		}

		if err := syscall.Close(fd); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkOpenNoApprover(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/donotmatch"`,
	}

	benchmarkOpenSameFile(b, false, rule)
}

func BenchmarkOpenWithApprover(b *testing.B) {
	rule := &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/donotmatch"`,
	}

	benchmarkOpenSameFile(b, true, rule)
}

func BenchmarkOpenNoKprobe(b *testing.B) {
	benchmarkOpenSameFile(b, false)
}

func createFolder(current string, filesPerFolder, maxDepth int) error {
	os.MkdirAll(current, 0777)

	for i := 0; i < filesPerFolder; i++ {
		f, err := os.Create(path.Join(current, fmt.Sprintf("file%d", i)))
		if err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
	}

	if maxDepth > 0 {
		if err := createFolder(path.Join(current, fmt.Sprintf("dir%d", maxDepth)), filesPerFolder, maxDepth-1); err != nil {
			return err
		}
	}

	return nil
}

func benchmarkFind(b *testing.B, filesPerFolder, maxDepth int, rules ...*rules.RuleDefinition) {
	test, err := newTestModule(nil, rules, testOpts{})
	if err != nil {
		b.Fatal(err)
	}
	defer test.Close()

	if err := createFolder(test.Root(), filesPerFolder, maxDepth); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		findCmd := exec.Command("/usr/bin/find", test.Root())
		if err := findCmd.Run(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFind(b *testing.B) {
	benchmarkFind(b, 128, 8, &rules.RuleDefinition{
		ID:         "test_rule",
		Expression: `open.filename == "{{.Root}}/donotmatch"`,
	})
}

func BenchmarkFindNoKprobe(b *testing.B) {
	benchmarkFind(b, 128, 8)
}
