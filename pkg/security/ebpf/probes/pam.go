// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package probes holds probes related files
package probes

import (
	"bufio"
	"bytes"
	"context"
	"debug/elf"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	manager "github.com/DataDog/ebpf-manager"
)

// === DEBUG ADDITIONS BEGIN ===
var pamDebug = true

func debugf(format string, a ...any) {
	if pamDebug {
		fmt.Fprintf(os.Stderr, "[pam-debug] "+format+"\n", a...)
	}
}

func readFileTrim(p string) string {
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func runCmd(timeout time.Duration, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	var errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("%s %v failed: %w (stderr: %s)", name, args, err, errb.String())
	}
	return out.String(), nil
}

func dumpSystemContext() {
	if !pamDebug {
		return
	}
	debugf("=== system context ===")
	if out, err := runCmd(2*time.Second, "uname", "-a"); err == nil {
		debugf("uname: %s", strings.TrimSpace(out))
	} else {
		debugf("uname error: %v", err)
	}
	debugf("perf_event_paranoid: %s", readFileTrim("/proc/sys/kernel/perf_event_paranoid"))
	debugf("kptr_restrict: %s", readFileTrim("/proc/sys/kernel/kptr_restrict"))
	// Secure Boot/Lockdown hints
	// On Ubuntu, lockdown state might appear here:
	if s := readFileTrim("/sys/kernel/security/lockdown"); s != "" {
		debugf("lockdown: %s", s)
	}
	// simple SB hint: efivars present?
	if _, err := os.Stat("/sys/firmware/efi/efivars"); err == nil {
		debugf("EFI vars present (Secure Boot possibly enabled)")
	}
}

func dumpLdconfigPamLines() {
	if !pamDebug {
		return
	}
	out, err := runCmd(2*time.Second, "ldconfig", "-p")
	if err != nil {
		debugf("ldconfig -p error: %v", err)
		return
	}
	lines := []string{}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "libpam.so") {
			lines = append(lines, l)
		}
	}
	debugf("ldconfig -p (filtered libpam):\n%s", strings.Join(lines, "\n"))
}

func elfInfoAndSymbols(path string, syms ...string) {
	if !pamDebug || path == "" {
		return
	}
	f, err := elf.Open(path)
	if err != nil {
		debugf("ELF open error for %s: %v", path, err)
		return
	}
	defer f.Close()
	debugf("ELF: %s, Class=%v, Data=%v, OSABI=%v, Type=%v, Machine=%v",
		path, f.FileHeader.Class, f.FileHeader.Data, f.FileHeader.OSABI, f.FileHeader.Type, f.FileHeader.Machine)

	// Prepare symbol matcher (accept versioned names like pam_start@@LIBPAM_1.0)
	match := func(name, target string) bool {
		return name == target || strings.HasPrefix(name, target+"@@") || strings.HasPrefix(name, target+"@")
	}

	checkTable := func(table string, get func() ([]elf.Symbol, error)) {
		symsList, err := get()
		if err != nil {
			debugf("%s symbols error: %v", table, err)
			return
		}
		for _, want := range syms {
			var hits []string
			for _, s := range symsList {
				if match(s.Name, want) && s.Info == elf.ST_INFO(elf.STB_GLOBAL, elf.STT_FUNC) {
					hits = append(hits, fmt.Sprintf("%s=0x%x", s.Name, s.Value))
				}
			}
			if len(hits) > 0 {
				debugf("%s: found %s -> %s", table, want, strings.Join(hits, ", "))
			} else {
				debugf("%s: NOT found %s", table, want)
			}
		}
	}

	if f.Section(".dynsym") != nil {
		checkTable(".dynsym", f.DynamicSymbols)
	} else {
		debugf(".dynsym: not present")
	}
	if f.Section(".symtab") != nil {
		checkTable(".symtab", f.Symbols)
	} else {
		debugf(".symtab: not present")
	}
}

func scanProcForPam(maxPIDs int) []string {
	if !pamDebug {
		return nil
	}
	entries, _ := os.ReadDir("/proc")
	var ret []string
	count := 0
	for _, e := range entries {
		if count >= maxPIDs {
			break
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		maps := fmt.Sprintf("/proc/%d/maps", pid)
		f, err := os.Open(maps)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			// look for any absolute path ending with libpam.so*
			if i := strings.LastIndex(line, "/"); i >= 0 {
				path := line[i:]
				if strings.Contains(path, "libpam.so") {
					// extract path token (last column)
					fields := strings.Fields(line)
					if len(fields) >= 6 {
						p := fields[len(fields)-1]
						if filepath.IsAbs(p) && strings.Contains(filepath.Base(p), "libpam.so") {
							ret = append(ret, fmt.Sprintf("pid=%d -> %s", pid, p))
							count++
							break
						}
					}
				}
			}
		}
		f.Close()
	}
	return ret
}

func explainIfSuspicious(path string) {
	if !pamDebug {
		return
	}
	if path == "" {
		debugf("No libpam path resolved. On Ubuntu 24.10, ensure libpam is installed: `apt-cache policy libpam0g`.")
		return
	}
	st, err := os.Stat(path)
	if err != nil {
		debugf("stat(%s) error: %v", path, err)
		return
	}
	if st.Mode()&os.ModeSymlink != 0 {
		if real, err := filepath.EvalSymlinks(path); err == nil {
			debugf("%s is a symlink -> %s", path, real)
		}
	}
	// inode
	if stat, ok := st.Sys().(*syscall.Stat_t); ok {
		debugf("file inode: dev=%d ino=%d mode=%#o size=%d", stat.Dev, stat.Ino, st.Mode().Perm(), st.Size())
	}
	// hint about mismatched inode:
	debugf("If the probe attaches but never fires, verify the *same inode* as mapped by target process (/proc/<pid>/maps).")
}

// === DEBUG ADDITIONS END ===

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
		// === DEBUG ADDITIONS ===
		debugf("ldconfig -p failed: %v", err)
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
		// === DEBUG ADDITIONS ===
		debugf("no libpam found in ldconfig output")
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

	// === DEBUG ADDITIONS ===
	debugf("ldconfig candidates: %v", cands)

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
		// === DEBUG ADDITIONS ===
		debugf("no libpam in common dirs")
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

	// === DEBUG ADDITIONS ===
	debugf("common dir candidates: %v", cands)

	return cands[0]
}

var libPamPath = getPamLibPath()

// === DEBUG ADDITIONS BEGIN ===
// Call this once early (e.g., from init or before starting the manager) to see context.
func init() {
	if pamDebug {
		dumpSystemContext()
		// show ldconfig filtered lines
		dumpLdconfigPamLines()
	}
	if libPamPath == "" && pamDebug {
		debugf("libPamPath is empty – probes will not attach")
	} else if pamDebug {
		debugf("libPamPath selected: %s", libPamPath)
		explainIfSuspicious(libPamPath)
		elfInfoAndSymbols(libPamPath, "pam_start", "pam_set_item")
		// sample a few PIDs to see which libpam inode is mapped (helps detect inode mismatch)
		if rows := scanProcForPam(5); len(rows) > 0 {
			debugf("sampled /proc/*/maps entries with libpam:\n%s", strings.Join(rows, "\n"))
		} else {
			debugf("no running process currently mapping libpam (that's OK if you're testing locally).")
		}
	}
}

// === DEBUG ADDITIONS END ===

func getPamProbes() []*manager.Probe {
	// === DEBUG ADDITIONS ===
	if pamDebug {
		debugf("building PAM probes with BinaryPath=%q", libPamPath)
		if libPamPath == "" {
			debugf("WARNING: empty BinaryPath – returning probes but attach will fail")
		}
	}

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

// OPTIONAL: helper you can call where you start the manager to dump attach errors if ebpf-manager exposes them.
// If your attach logic is elsewhere, ignore this.
func LogProbeAttachErrors(w io.Writer, probes []*manager.Probe, errs map[string]error) {
	if !pamDebug || len(errs) == 0 {
		return
	}
	fmt.Fprintln(w, "[pam-debug] === probe attach errors ===")
	for _, p := range probes {
		key := fmt.Sprintf("%s:%s", p.ProbeIdentificationPair.UID, p.ProbeIdentificationPair.EBPFFuncName)
		if err, ok := errs[key]; ok && err != nil {
			fmt.Fprintf(w, "[pam-debug] %s path=%s -> %v\n", key, p.BinaryPath, err)
		}
	}
}
