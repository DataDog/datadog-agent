// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build functionaltests
// +build functionaltests

package tests

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"golang.org/x/sys/unix"
)

//go:embed syscall_tester/bin
var syscallTesterFS embed.FS

func loadSyscallTester(t *testing.T, test *testModule, binary string) (string, error) {
	var uname unix.Utsname
	if err := unix.Uname(&uname); err != nil {
		return "", fmt.Errorf("couldn't resolve arch: %w", err)
	}

	testerBin, err := syscallTesterFS.ReadFile(fmt.Sprintf("syscall_tester/bin/%s", binary))
	if err != nil {
		return "", err
	}

	perm := 0o700
	binPath, _, _ := test.CreateWithOptions(binary, -1, -1, perm)

	f, err := os.OpenFile(binPath, os.O_WRONLY|os.O_CREATE, os.FileMode(perm))
	if err != nil {
		return "", err
	}

	if _, err = f.Write(testerBin); err != nil {
		f.Close()
		return "", err
	}
	f.Close()

	if err := checkSyscallTester(t, binPath); err != nil {
		return "", err
	}

	return binPath, nil
}

func checkSyscallTester(t *testing.T, path string) error {
	t.Helper()
	sideTester := exec.Command(path, "check")
	if _, err := sideTester.CombinedOutput(); err != nil {
		return fmt.Errorf("cannot run syscall tester check: %w", err)
	}
	return nil
}

func runSyscallTesterFunc(t *testing.T, path string, args ...string) error {
	t.Helper()
	sideTester := exec.Command(path, args...)
	output, err := sideTester.CombinedOutput()

	if err != nil {
		t.Error(err)
		output := string(output)
		if output != "" {
			t.Error(output)
		}
	}
	return err
}
