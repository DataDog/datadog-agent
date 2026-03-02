// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"bytes"
	"os"
	"runtime"
	"strconv"
	"testing"
)

func oldWithAllProcs(procRoot string, fn func(int) error) error {
	files, err := os.ReadDir(procRoot)
	if err != nil {
		return err
	}

	for _, f := range files {
		if !f.IsDir() || f.Name() == "." || f.Name() == ".." {
			continue
		}

		var pid int
		if pid, err = strconv.Atoi(f.Name()); err != nil {
			continue
		}

		if err = fn(pid); err != nil {
			return err
		}
	}
	return nil
}

func BenchmarkOldWithAllProcs(b *testing.B) {

	var pids []int
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pids := []int{}
		oldWithAllProcs("/proc", func(pid int) error {
			pids = append(pids, pid)
			return nil
		})
	}
	runtime.KeepAlive(pids)
}

func BenchmarkWithAllProcs(b *testing.B) {
	var pids []int

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pids = []int{}
		WithAllProcs("/proc", func(pid int) error {
			pids = append(pids, pid)
			return nil
		})
	}
	runtime.KeepAlive(pids)
}

func BenchmarkAllPidsProcs(b *testing.B) {
	var pids []int

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pids, _ = AllPidsProcs("/proc")
	}
	runtime.KeepAlive(pids)
}

func TestGetEnvVariableFromBuffer(t *testing.T) {
	cases := []struct {
		name     string
		contents string
		envVar   string
		expected string
	}{
		{
			name:     "NonExistent",
			contents: "PATH=/usr/bin\x00HOME=/home/user\x00",
			envVar:   "NONEXISTENT",
			expected: "",
		},
		{
			name:     "Exists",
			contents: "PATH=/usr/bin\x00MY_VAR=myvar\x00HOME=/home/user\x00",
			envVar:   "MY_VAR",
			expected: "myvar",
		},
		{
			name:     "Empty",
			contents: "PATH=/usr/bin\x00MY_VAR=\x00HOME=/home/user\x00",
			envVar:   "MY_VAR",
			expected: "",
		},
		{
			name:     "PrefixVarNotSelected",
			contents: "PATH=/usr/bin\x00MY_VAR_BUT_NOT_THIS=nope\x00MY_VAR=myvar\x00HOME=/home/user\x00",
			envVar:   "MY_VAR",
			expected: "myvar",
		},
		{
			name:     "LastVarWithNoTrailingNull",
			contents: "PATH=/usr/bin\x00MY_VAR=myvar",
			envVar:   "MY_VAR",
			expected: "myvar",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual := getEnvVariableFromBuffer(bytes.NewBufferString(tc.contents), tc.envVar)
			if actual != tc.expected {
				t.Fatalf("Expected %s, got %s", tc.expected, actual)
			}
		})
	}
}

func TestAllPidsProcs(t *testing.T) {
	// Test with /proc which should return at least our process
	pids, err := AllPidsProcs("/proc")
	if err != nil {
		t.Fatalf("AllPidsProcs failed: %v", err)
	}

	if len(pids) == 0 {
		t.Fatal("Expected at least one PID")
	}

	// Check that our PID is in the list
	ourPid := os.Getpid()
	found := false
	for _, pid := range pids {
		if pid == ourPid {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Our PID %d not found in pids list", ourPid)
	}
}

func TestAllPidsProcsInvalidPath(t *testing.T) {
	_, err := AllPidsProcs("/nonexistent/path")
	if err == nil {
		t.Fatal("Expected error for nonexistent path")
	}
}

func TestWithAllProcs(t *testing.T) {
	var pids []int
	err := WithAllProcs("/proc", func(pid int) error {
		pids = append(pids, pid)
		return nil
	})

	if err != nil {
		t.Fatalf("WithAllProcs failed: %v", err)
	}

	if len(pids) == 0 {
		t.Fatal("Expected at least one PID")
	}
}

func TestWithAllProcsInvalidPath(t *testing.T) {
	err := WithAllProcs("/nonexistent/path", func(pid int) error {
		return nil
	})

	if err == nil {
		t.Fatal("Expected error for nonexistent path")
	}
}

func TestScanNullStrings(t *testing.T) {
	cases := []struct {
		name     string
		data     []byte
		atEOF    bool
		advance  int
		token    []byte
		hasToken bool
	}{
		{
			name:     "empty at EOF",
			data:     []byte{},
			atEOF:    true,
			advance:  0,
			token:    nil,
			hasToken: false,
		},
		{
			name:     "null terminated string",
			data:     []byte("hello\x00world"),
			atEOF:    false,
			advance:  6,
			token:    []byte("hello"),
			hasToken: true,
		},
		{
			name:     "no null not at EOF",
			data:     []byte("hello"),
			atEOF:    false,
			advance:  0,
			token:    nil,
			hasToken: false,
		},
		{
			name:     "no null at EOF",
			data:     []byte("hello"),
			atEOF:    true,
			advance:  5,
			token:    []byte("hello"),
			hasToken: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			advance, token, err := scanNullStrings(tc.data, tc.atEOF)
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}
			if advance != tc.advance {
				t.Errorf("Expected advance %d, got %d", tc.advance, advance)
			}
			if tc.hasToken {
				if token == nil {
					t.Error("Expected token, got nil")
				} else if !bytes.Equal(token, tc.token) {
					t.Errorf("Expected token %q, got %q", tc.token, token)
				}
			} else {
				if token != nil {
					t.Errorf("Expected nil token, got %q", token)
				}
			}
		})
	}
}

func TestGetProcessEnvVariable(t *testing.T) {
	// Test getting PATH from our own process
	pid := os.Getpid()
	path, err := GetProcessEnvVariable(pid, "/proc", "PATH")
	if err != nil {
		t.Fatalf("GetProcessEnvVariable failed: %v", err)
	}
	// PATH should not be empty
	if path == "" {
		t.Log("PATH env variable is empty, which is unusual but not necessarily wrong")
	}
}

func TestGetProcessEnvVariableInvalidPid(t *testing.T) {
	_, err := GetProcessEnvVariable(-999999, "/proc", "PATH")
	if err == nil {
		t.Fatal("Expected error for invalid PID")
	}
}

func TestProcessExists(t *testing.T) {
	// Test with our own PID - should exist
	ourPid := os.Getpid()
	if !ProcessExists(ourPid) {
		t.Errorf("ProcessExists returned false for our own PID %d", ourPid)
	}

	// Test with invalid PID - should not exist
	if ProcessExists(-1) {
		t.Error("ProcessExists returned true for invalid PID -1")
	}
}
