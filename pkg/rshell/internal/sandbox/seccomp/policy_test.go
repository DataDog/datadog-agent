// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package seccomp

import (
	"slices"
	"strings"
	"testing"

	elasticseccomp "github.com/elastic/go-seccomp-bpf"
)

func TestSyscallsForArchitecture(t *testing.T) {
	amd64, err := syscallsForArchitecture("amd64", []string{"mount", "fork", "iopl"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"fork", "iopl", "mount"} {
		if !slices.Contains(amd64, name) {
			t.Errorf("amd64 policy is missing %q", name)
		}
	}

	arm64, err := syscallsForArchitecture("arm64", []string{"mount", "fork", "iopl"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(arm64, []string{"mount"}) {
		t.Fatalf("arm64 policy = %v, want [mount]", arm64)
	}
}

func TestDefaultDenylistIsValidForSupportedArchitectures(t *testing.T) {
	for _, architecture := range supportedArchitectures {
		t.Run(architecture, func(t *testing.T) {
			group, err := syscallGroupForArchitecture(architecture, DefaultDenylist())
			if err != nil {
				t.Fatal(err)
			}
			if len(group.Names) == 0 {
				t.Fatal("default policy has no unconditional syscall rules")
			}
			if len(group.NamesWithCondtions) != 1 {
				t.Fatalf("conditional rules = %v, want exactly clone", group.NamesWithCondtions)
			}
			clone := group.NamesWithCondtions[0]
			if clone.Name != "clone" || len(clone.Conditions) != 1 {
				t.Fatalf("clone rule = %#v", clone)
			}
			wantCloneFlags, err := goRuntimeCloneFlagsForArchitecture(architecture)
			if err != nil {
				t.Fatal(err)
			}
			condition := clone.Conditions[0]
			if condition.Argument != 0 || condition.Operation != elasticseccomp.NotEqual || condition.Value != wantCloneFlags {
				t.Fatalf("clone condition = %#v", condition)
			}
		})
	}
}

func TestGoRuntimeCloneFlagsForArchitecture(t *testing.T) {
	amd64, err := goRuntimeCloneFlagsForArchitecture("amd64")
	if err != nil {
		t.Fatal(err)
	}
	if amd64 != baseGoRuntimeCloneFlags|cloneSetTLS {
		t.Fatalf("amd64 clone flags = %#x", amd64)
	}

	arm64, err := goRuntimeCloneFlagsForArchitecture("arm64")
	if err != nil {
		t.Fatal(err)
	}
	if arm64 != baseGoRuntimeCloneFlags {
		t.Fatalf("arm64 clone flags = %#x", arm64)
	}
}

func TestSyscallsForArchitectureRejectsInvalidPolicy(t *testing.T) {
	tests := []struct {
		name         string
		architecture string
		denied       []string
		want         string
	}{
		{name: "unsupported architecture", architecture: "riscv64", denied: []string{"mount"}, want: "unsupported arch"},
		{name: "empty list", architecture: "amd64", want: "must not be empty"},
		{name: "empty name", architecture: "amd64", denied: []string{""}, want: "empty syscall name"},
		{name: "unknown name", architecture: "amd64", denied: []string{"definitely_not_a_syscall"}, want: "unknown syscall"},
		{name: "duplicate", architecture: "amd64", denied: []string{"mount", "mount"}, want: "duplicate syscall"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := syscallsForArchitecture(test.architecture, test.denied)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want substring %q", err, test.want)
			}
		})
	}
}
