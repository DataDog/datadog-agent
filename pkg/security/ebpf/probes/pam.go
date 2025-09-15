// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	manager "github.com/DataDog/ebpf-manager"
)

// getPamLibPath returns the absolute path to the PAM shared library by
// parsing the system linker cache (ldconfig -p). It prefers the SONAME
// libpam.so.0 but will fall back to any libpam.so* found in common lib dirs.
// It returns an empty string if nothing is found.
func getPamLibPath() string {
	if p, _ := pamFromLdconfig(); p != "" {
		fmt.Printf("pamFromLdconfig: %s\n", p)
		return p
	}
	if p := pamFromCommonDirs(); p != "" {
		fmt.Printf("pamFromCommonDirs: %s\n", p)
		return p
	}
	return ""
}

// pamFromLdconfig queries `ldconfig -p` and parses results for libpam.so*
func pamFromLdconfig() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "ldconfig", "-p").Output()
	if err != nil {
		return "", err
	}

	// Example lines:
	//     libpam.so.0 (libc6,x86-64) => /lib/x86_64-linux-gnu/libpam.so.0
	//     libpam.so (libc6,x86-64)   => /usr/lib/x86_64-linux-gnu/libpam.so
	re := regexp.MustCompile(`^\s*(libpam\.so(\.[0-9]+)?)\s+\(.*?\)\s*=>\s*(\S+)\s*$`)
	type cand struct {
		name string
		path string
	}
	var cands []cand

	for _, line := range bytes.Split(out, []byte{'\n'}) {
		m := re.FindSubmatch(line)
		if len(m) == 0 {
			continue
		}
		name := string(m[1]) // libpam.so or libpam.so.X
		path := string(m[3])
		if fileExists(path) {
			cands = append(cands, cand{name, path})
		}
	}

	if len(cands) == 0 {
		return "", errors.New("no libpam in ldconfig cache")
	}

	// Rank candidates:
	//  1) prefer libpam.so.<number> (runtime SONAME) over plain libpam.so (dev symlink)
	//  2) if multiple versioned, pick the highest numeric suffix
	sort.SliceStable(cands, func(i, j int) bool {
		vi := versionWeight(cands[i].name)
		vj := versionWeight(cands[j].name)
		if vi != vj {
			return vi > vj
		}
		// tie-breaker: longer path (often the real file, not a symlink), then lexicographically
		if len(cands[i].path) != len(cands[j].path) {
			return len(cands[i].path) > len(cands[j].path)
		}
		return cands[i].path > cands[j].path
	})

	return cands[0].path, nil
}

// versionWeight assigns a weight based on the SONAME:
//
//	libpam.so.0 -> 1000 + 0
//	libpam.so.1 -> 1000 + 1
//	libpam.so   -> 0
func versionWeight(name string) int {
	if !strings.HasPrefix(name, "libpam.so") {
		return 0
	}
	if name == "libpam.so" {
		return 0
	}
	// libpam.so.<n>
	parts := strings.Split(name, ".")
	if len(parts) >= 3 {
		if v := atoiSafe(parts[len(parts)-1]); v >= 0 {
			return 1000 + v
		}
	}
	return 1 // any other weird suffix still beats plain .so
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return -1
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// pamFromCommonDirs does a very small, safe search in standard lib paths.
// This is a fallback for systems without ldconfig -p (e.g., some Alpine images).
func pamFromCommonDirs() string {
	// Add/trim directories as needed for your environment.
	dirs := []string{
		"/lib", "/usr/lib", "/lib64", "/usr/lib64",
		"/lib/x86_64-linux-gnu", "/usr/lib/x86_64-linux-gnu",
		"/lib/aarch64-linux-gnu", "/usr/lib/aarch64-linux-gnu",
		"/lib/arm-linux-gnueabihf", "/usr/lib/arm-linux-gnueabihf",
		"/lib/powerpc64le-linux-gnu", "/usr/lib/powerpc64le-linux-gnu",
	}
	var cands []string

	for _, d := range dirs {
		paths, _ := filepath.Glob(filepath.Join(d, "libpam.so*"))
		for _, p := range paths {
			if fileExists(p) {
				cands = append(cands, p)
			}
		}
	}

	if len(cands) == 0 {
		return ""
	}

	// Prefer versioned .so.N over plain .so; then longest path.
	sort.SliceStable(cands, func(i, j int) bool {
		wi := 0
		wj := 0
		if strings.HasPrefix(filepath.Base(cands[i]), "libpam.so.") {
			wi = 1
		}
		if strings.HasPrefix(filepath.Base(cands[j]), "libpam.so.") {
			wj = 1
		}
		if wi != wj {
			return wi > wj
		}
		if len(cands[i]) != len(cands[j]) {
			return len(cands[i]) > len(cands[j])
		}
		return cands[i] > cands[j]
	})

	return cands[0]
}

var libPamPath = getPamLibPath()

func getPamProbes() []*manager.Probe {

	var pamProbes = []*manager.Probe{
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_pam_start",
			},
			BinaryPath: libPamPath,
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "rethook_pam_start",
			},
			BinaryPath: libPamPath,
		},
		{
			ProbeIdentificationPair: manager.ProbeIdentificationPair{
				UID:          SecurityAgentUID,
				EBPFFuncName: "hook_pam_set_item",
			},
			BinaryPath: libPamPath,
		},
	}

	return pamProbes
}
