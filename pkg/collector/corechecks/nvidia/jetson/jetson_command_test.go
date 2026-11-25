// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package nvidia

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	tegrastatsHelperRole  = "DD_JETSON_TEST_HELPER_PROCESS"
	tegrastatsHelperBound = 10 * time.Second
)

func TestMain(m *testing.M) {
	switch os.Getenv(tegrastatsHelperRole) {
	case "sudo-relay":
		os.Exit(fakeSudo(true))
	case "sudo-ignore":
		os.Exit(fakeSudo(false))
	case "tegrastats":
		os.Exit(fakeTegraStats())
	case "failure":
		fmt.Fprintln(os.Stderr, "boom")
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// fakeSudo owns the command it starts and relays SIGTERM in the normal case. The
// tegrastats process inherits stdout, so killing fake sudo alone doesn't close it.
func fakeSudo(relay bool) int {
	if len(os.Args) != 5 || os.Args[1] != "-n" {
		fmt.Fprintf(os.Stderr, "fake sudo: expected -n <command> --interval 500, got %q\n", os.Args[1:])
		return 1
	}

	signals := make(chan os.Signal, 1)
	if relay {
		signal.Notify(signals, syscall.SIGTERM)
		defer signal.Stop(signals)
	} else {
		signal.Ignore(syscall.SIGTERM)
	}

	ready := os.NewFile(3, "ready")
	stop := os.NewFile(4, "stop")
	child := exec.Command(os.Args[2], os.Args[3:]...)
	child.Env = append(os.Environ(), tegrastatsHelperRole+"=tegrastats")
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	child.ExtraFiles = []*os.File{ready, stop}
	err := child.Start()
	_ = ready.Close()
	_ = stop.Close()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	if relay {
		go func() {
			<-signals
			_ = child.Process.Signal(syscall.SIGTERM)
		}()
	}
	_ = child.Wait()
	return child.ProcessState.ExitCode()
}

func fakeTegraStats() int {
	if len(os.Args) != 3 || os.Args[1] != "--interval" || os.Args[2] != "500" {
		fmt.Fprintf(os.Stderr, "fake tegrastats: expected --interval 500, got %q\n", os.Args[1:])
		return 1
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	defer signal.Stop(signals)
	ready := os.NewFile(3, "ready")
	stop := os.NewFile(4, "stop")
	defer stop.Close()
	fmt.Println(nanoSample)
	_, err := ready.Write([]byte{1})
	_ = ready.Close()
	if err != nil {
		return 1
	}

	stopped := make(chan struct{})
	go func() {
		// Test-owned write end closes even when a regression leaves us orphaned.
		_, _ = io.Copy(io.Discard, stop)
		close(stopped)
	}()
	select {
	case <-signals:
		return 143
	case <-stopped:
		return 2
	}
}

func TestSudoTegraStatsCommand(t *testing.T) {
	cmd := sudoTegraStatsCommand(context.Background(), "sudo", defaultTegraStatsPath, []string{"--interval", "500"})
	assert.Equal(t, []string{"sudo", "-n", defaultTegraStatsPath, "--interval", "500"}, cmd.Args)
	require.NotNil(t, cmd.SysProcAttr)
	assert.True(t, cmd.SysProcAttr.Setpgid, "older sudo versions won't forward signals from their own process group")
	assert.NotNil(t, cmd.Cancel)
	assert.Equal(t, tegraStatsWaitDelay, cmd.WaitDelay)
}

func TestSudoTegraStatsCancellation(t *testing.T) {
	for _, tc := range []struct {
		name         string
		role         string
		wantExitCode int
	}{
		{name: "sudo relays termination", role: "sudo-relay", wantExitCode: 143},
		{name: "sudo does not relay termination", role: "sudo-ignore", wantExitCode: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			exe, err := os.Executable()
			require.NoError(t, err)
			readyReader, readyWriter, err := os.Pipe()
			require.NoError(t, err)
			t.Cleanup(func() { _ = readyReader.Close(); _ = readyWriter.Close() })
			stopReader, stopWriter, err := os.Pipe()
			require.NoError(t, err)
			t.Cleanup(func() { _ = stopReader.Close(); _ = stopWriter.Close() })

			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)
			cmd := sudoTegraStatsCommand(ctx, exe, exe, []string{"--interval", "500"})
			cmd.Env = append(os.Environ(), tegrastatsHelperRole+"="+tc.role)
			cmd.ExtraFiles = []*os.File{readyWriter, stopReader}
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			require.NoError(t, cmd.Start())
			_ = readyWriter.Close()
			_ = stopReader.Close()

			var waitErr error
			done := make(chan struct{})
			go func() {
				waitErr = cmd.Wait()
				close(done)
			}()
			t.Cleanup(func() {
				_ = stopWriter.Close()
				cancel()
				select {
				case <-done:
				case <-time.After(tegrastatsHelperBound):
					_ = cmd.Process.Kill()
					t.Error("sudo helper did not exit after cleanup")
				}
			})

			// Cancel only after the child wrote stdout and installed its TERM handler.
			require.NoError(t, readyReader.SetReadDeadline(time.Now().Add(tegrastatsHelperBound)))
			var marker [1]byte
			_, err = io.ReadFull(readyReader, marker[:])
			require.NoError(t, err, "fake tegrastats did not become ready")
			cancel()
			select {
			case <-done:
			case <-time.After(tegrastatsHelperBound):
				t.Fatal("command is stuck waiting for the tegrastats stdout pipe")
			}

			var exitErr *exec.ExitError
			require.ErrorAs(t, waitErr, &exitErr, "helper stderr: %s", stderr.String())
			assert.Equal(t, tc.wantExitCode, exitErr.ExitCode())
			if tc.wantExitCode == -1 {
				assert.Equal(t, syscall.SIGKILL, exitErr.Sys().(syscall.WaitStatus).Signal())
			}
			assert.Contains(t, stdout.String(), nanoSample)
			assert.NoError(t, checkTegraStatsResult(stdout.Bytes(), waitErr))
		})
	}
}

func TestCheckTegraStatsResult(t *testing.T) {
	exe, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.CommandContext(t.Context(), exe)
	cmd.Env = append(os.Environ(), tegrastatsHelperRole+"=failure")
	_, err = cmd.Output()
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)

	otherErr := errors.New("unexpected launch error")
	tests := []struct {
		name      string
		hasOutput bool
		err       error
		wantErr   error
		wantText  string
	}{
		{name: "success", hasOutput: true},
		{name: "exit error with output", hasOutput: true, err: exitErr},
		{name: "exit error without output", err: exitErr, wantErr: exitErr, wantText: "Stderr: boom"},
		{name: "deadline with output", hasOutput: true, err: context.DeadlineExceeded},
		{name: "deadline without output", err: context.DeadlineExceeded, wantErr: context.DeadlineExceeded, wantText: "tegrastats did not produce any output"},
		{name: "wait delay with output", hasOutput: true, err: exec.ErrWaitDelay},
		{name: "wait delay without output", err: exec.ErrWaitDelay, wantErr: exec.ErrWaitDelay, wantText: "tegrastats did not produce any output"},
		{name: "unrelated error with output", hasOutput: true, err: otherErr, wantErr: otherErr},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var output []byte
			if tc.hasOutput {
				output = []byte(nanoSample)
			}
			err := checkTegraStatsResult(output, tc.err)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
			if tc.wantText != "" {
				assert.ErrorContains(t, err, tc.wantText)
			}
		})
	}
}
