// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package seccomp

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"testing"
	"time"

	elasticseccomp "github.com/elastic/go-seccomp-bpf"
	"golang.org/x/sys/unix"
)

const seccompHelperEnvironment = "RSHELL_SECCOMP_TEST_HELPER"

func TestRestrictReturnsEPERMInSubprocess(t *testing.T) {
	if os.Getenv(seccompHelperEnvironment) == "custom" {
		runSeccompHelper()
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestRestrictReturnsEPERMInSubprocess$")
	command.Env = append(os.Environ(), seccompHelperEnvironment+"=custom")
	output, err := command.CombinedOutput()
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) && exitErr.ExitCode() == 77 {
		t.Skipf("seccomp unavailable: %s", output)
	}
	if err != nil {
		t.Fatalf("seccomp helper failed: %v\n%s", err, output)
	}
}

func TestRestrictDefaultInSubprocess(t *testing.T) {
	if os.Getenv(seccompHelperEnvironment) == "default" {
		runDefaultSeccompHelper()
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestRestrictDefaultInSubprocess$")
	command.Env = append(os.Environ(), seccompHelperEnvironment+"=default")
	output, err := command.CombinedOutput()
	if exitErr := (*exec.ExitError)(nil); errors.As(err, &exitErr) && exitErr.ExitCode() == 77 {
		t.Skipf("seccomp unavailable: %s", output)
	}
	if err != nil {
		t.Fatalf("default seccomp helper failed: %v\n%s", err, output)
	}
}

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

func runSeccompHelper() {
	if !elasticseccomp.Supported() {
		fmt.Fprintln(os.Stderr, "seccomp is not supported by this kernel")
		os.Exit(77)
	}
	if err := Restrict([]string{"getpid"}); err != nil {
		fmt.Fprintf(os.Stderr, "restrict: %v\n", err)
		os.Exit(1)
	}

	_, _, errno := syscall.RawSyscall(syscall.SYS_GETPID, 0, 0, 0)
	if errno != syscall.EPERM {
		fmt.Fprintf(os.Stderr, "getpid errno = %v, want %v\n", errno, syscall.EPERM)
		os.Exit(1)
	}
	_, _, errno = syscall.RawSyscall(syscall.SYS_GETPPID, 0, 0, 0)
	if errno != 0 {
		fmt.Fprintf(os.Stderr, "allowed getppid errno = %v, want 0\n", errno)
		os.Exit(1)
	}
}

func runDefaultSeccompHelper() {
	if !elasticseccomp.Supported() {
		fmt.Fprintln(os.Stderr, "seccomp is not supported by this kernel")
		os.Exit(77)
	}
	if err := RestrictDefault(); err != nil {
		fmt.Fprintf(os.Stderr, "restrict default: %v\n", err)
		os.Exit(1)
	}

	verifyGoRuntimeThreadCreation()

	assertRawSyscallErrno("clone", syscall.SYS_CLONE, uintptr(unix.CLONE_THREAD), 0, 0, syscall.EPERM)
	assertRawSyscallErrno("execve", syscall.SYS_EXECVE, 0, 0, 0, syscall.EPERM)
	assertRawSyscallErrno("prctl", syscall.SYS_PRCTL, uintptr(unix.PR_GET_DUMPABLE), 0, 0, syscall.EPERM)
	assertRawSyscallErrno("ioctl", syscall.SYS_IOCTL, ^uintptr(0), 0, 0, syscall.EPERM)

	// setresuid must remain available to the helper's controlled elevation
	// callback. This no-op form changes only the effective UID to its current
	// value and leaves the real and saved UIDs untouched.
	minusOne := ^uintptr(0)
	_, _, errno := syscall.RawSyscall(syscall.SYS_SETRESUID, minusOne, uintptr(os.Geteuid()), minusOne)
	if errno != 0 {
		fmt.Fprintf(os.Stderr, "allowed setresuid errno = %v, want 0\n", errno)
		os.Exit(1)
	}
}

func verifyGoRuntimeThreadCreation() {
	if raceEnabled {
		// The race runtime uses cgo's pthread_create path, which prefers clone3.
		// Production rshell binaries are pure Go, and the default policy denies
		// clone3 while allowing only the Go runtime's exact clone flags. Keep the
		// syscall assertions below active under -race; exercise runtime thread
		// creation in the ordinary (non-race) test binary.
		return
	}

	// Starting locked goroutines forces the runtime to use its exact clone
	// flags to provision additional OS threads after the filter is active.
	const threadCount = 8
	ready := make(chan struct{}, threadCount)
	release := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(threadCount)
	for range threadCount {
		go func() {
			defer wait.Done()
			runtime.LockOSThread()
			defer runtime.UnlockOSThread()
			ready <- struct{}{}
			<-release
		}()
	}
	for range threadCount {
		select {
		case <-ready:
		case <-time.After(5 * time.Second):
			fmt.Fprintln(os.Stderr, "timed out creating Go runtime threads")
			os.Exit(1)
		}
	}
	close(release)
	wait.Wait()
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

func assertRawSyscallErrno(name string, number, arg1, arg2, arg3 uintptr, want syscall.Errno) {
	_, _, errno := syscall.RawSyscall(number, arg1, arg2, arg3)
	if errno != want {
		fmt.Fprintf(os.Stderr, "%s errno = %v, want %v\n", name, errno, want)
		os.Exit(1)
	}
}
