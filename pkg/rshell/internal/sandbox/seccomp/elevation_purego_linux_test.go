// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && !cgo

package seccomp

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"

	elasticseccomp "github.com/elastic/go-seccomp-bpf"
)

func TestDeniedSyscallRemainsDeniedAfterSelectiveElevation(t *testing.T) {
	if os.Getenv(seccompHelperEnvironment) == "root-elevation" {
		runRootElevationSeccompHelper()
		return
	}
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		t.Skip("requires real and effective UID 0")
	}

	command := exec.Command(os.Args[0], "-test.run=^TestDeniedSyscallRemainsDeniedAfterSelectiveElevation$")
	command.Env = append(os.Environ(), seccompHelperEnvironment+"=root-elevation")
	output, err := command.CombinedOutput()
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) && exitErr.ExitCode() == 77 {
		t.Skipf("seccomp unavailable: %s", output)
	}
	if err != nil {
		t.Fatalf("root elevation seccomp helper failed: %v\n%s", err, output)
	}
}

func runRootElevationSeccompHelper() {
	if os.Getuid() != 0 || os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "requires real and effective UID 0")
		os.Exit(77)
	}
	if !elasticseccomp.Supported() {
		fmt.Fprintln(os.Stderr, "seccomp is not supported by this kernel")
		os.Exit(77)
	}

	minusOne := ^uintptr(0)
	const unprivilegedUID = 65534
	if _, _, errno := syscall.AllThreadsSyscall(syscall.SYS_SETRESUID, minusOne, unprivilegedUID, minusOne); errno != 0 {
		fmt.Fprintf(os.Stderr, "drop effective UID: %v\n", errno)
		os.Exit(1)
	}
	if os.Getuid() != 0 || os.Geteuid() != unprivilegedUID {
		fmt.Fprintf(os.Stderr, "dropped credentials = ruid %d euid %d\n", os.Getuid(), os.Geteuid())
		os.Exit(1)
	}

	if err := RestrictDefault(); err != nil {
		fmt.Fprintf(os.Stderr, "restrict default: %v\n", err)
		os.Exit(1)
	}
	if _, _, errno := syscall.AllThreadsSyscall(syscall.SYS_SETRESUID, minusOne, 0, minusOne); errno != 0 {
		fmt.Fprintf(os.Stderr, "restore effective UID 0: %v\n", errno)
		os.Exit(1)
	}
	if os.Geteuid() != 0 {
		fmt.Fprintf(os.Stderr, "effective UID = %d, want 0\n", os.Geteuid())
		os.Exit(1)
	}

	// Seccomp is monotonic and remains active after the controlled elevation.
	assertRawSyscallErrno("ioctl after elevation", syscall.SYS_IOCTL, ^uintptr(0), 0, 0, syscall.EPERM)
	assertRawSyscallErrno("fchmod after elevation", syscall.SYS_FCHMOD, ^uintptr(0), 0, 0, syscall.EPERM)

	if _, _, errno := syscall.AllThreadsSyscall(syscall.SYS_SETRESUID, minusOne, unprivilegedUID, minusOne); errno != 0 {
		fmt.Fprintf(os.Stderr, "drop effective UID after test: %v\n", errno)
		os.Exit(1)
	}
}
