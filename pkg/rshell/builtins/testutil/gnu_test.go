// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFindGNUCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("reference executable fixtures require /bin/sh")
	}
	tests := []struct {
		name        string
		executables map[string]string
		want        string
	}{
		{name: "GNU default", executables: map[string]string{"wc": "printf 'wc (GNU coreutils) 9.7'"}, want: "wc"},
		{name: "Ubuntu GNU alongside uutils", executables: map[string]string{"wc": "printf 'wc (uutils coreutils) 0.8.0'", "gnuwc": "printf 'wc (GNU coreutils) 9.7'"}, want: "gnuwc"},
		{name: "Homebrew GNU alongside BSD", executables: map[string]string{"wc": "exit 1", "gwc": "printf 'wc (GNU coreutils) 9.7'"}, want: "gwc"},
		{name: "reject non-GNU default", executables: map[string]string{"wc": "printf 'wc (uutils coreutils) 0.8.0'"}},
		{name: "reject failed version probe", executables: map[string]string{"wc": "printf 'wc (GNU coreutils) 9.7'; exit 1"}},
		{name: "missing reference"},
		{name: "reject old word semantics", executables: map[string]string{"wc": "printf 'wc (GNU coreutils) 9.4'"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			for name, body := range test.executables {
				if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("PATH", dir)
			got, err := findGNUCommand(t.Context(), "wc")
			if test.want == "" {
				if err == nil || !strings.Contains(err.Error(), "GNU coreutils wc is required") {
					t.Fatalf("expected actionable missing-reference error; path=%q, error=%v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := filepath.Join(dir, test.want); got != want {
				t.Fatalf("resolved %q, want %q", got, want)
			}
		})
	}
}

func TestRunGNUInDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("reference executable fixture requires /bin/sh")
	}
	dir := t.TempDir()
	command := filepath.Join(dir, "reference")
	script := "#!/bin/sh\nprintf '%s|%s' \"$LC_ALL\" \"$1\"\nprintf 'diagnostic' >&2\nexit 3\n"
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	output, code := RunGNUInDir(t, dir, command, "argument with spaces")
	if output != "C.UTF-8|argument with spaces" || code != 3 {
		t.Fatalf("reference returned (%q, %d)", output, code)
	}
}
