// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package du_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"mvdan.cc/sh/v3/syntax"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins/testutil"
)

// FuzzDuFlags fuzzes the flag-parsing surface of du with arbitrary
// command-line strings. The seed corpus exercises every supported flag
// plus several rejected/unknown flags. The fuzz job verifies that no
// input triggers a panic, hang, or exit code outside {0, 1}.
func FuzzDuFlags(f *testing.F) {
	// Source A — implementation edge cases (every supported flag).
	f.Add("du file.txt")
	f.Add("du -a file.txt")
	f.Add("du -s file.txt")
	f.Add("du -c file.txt")
	f.Add("du -d 0 file.txt")
	f.Add("du -d 1 file.txt")
	f.Add("du -d 100 file.txt")
	f.Add("du -d -1 file.txt") // negative depth: should reject
	f.Add("du -S file.txt")
	f.Add("du -L file.txt")
	f.Add("du -P file.txt")
	f.Add("du -L -P file.txt") // toggle precedence
	f.Add("du -P -L file.txt")
	f.Add("du -0 file.txt")
	f.Add("du -h file.txt")
	f.Add("du --si file.txt")
	f.Add("du -k file.txt")
	f.Add("du -m file.txt")
	f.Add("du -b file.txt")
	f.Add("du --apparent-size file.txt")
	f.Add("du --help")

	// Combined short flags.
	f.Add("du -ab file.txt")
	f.Add("du -sh file.txt")
	f.Add("du -ch file.txt")
	f.Add("du -ahS file.txt")

	// Mutual-exclusion paths.
	f.Add("du -s -a file.txt")
	f.Add("du -s -d 1 file.txt")

	// Source B — CVE/security history-inspired inputs.
	f.Add("du --files0-from=anything") // exfiltration risk → reject
	f.Add("du --exclude-from=anything")
	f.Add("du --exclude=*.o")
	f.Add("du -X file.txt")
	f.Add("du -B 1024 file.txt") // block-size: not implemented
	f.Add("du -t 1024 file.txt") // threshold: not implemented
	f.Add("du --inodes file.txt")
	f.Add("du --time file.txt")
	f.Add("du --time-style=iso file.txt")

	// Integer overflow inputs.
	f.Add("du -d 9223372036854775807 file.txt")  // MaxInt64
	f.Add("du -d 9223372036854775808 file.txt")  // MaxInt64+1
	f.Add("du -d 99999999999999999999 file.txt") // huge
	f.Add("du -d -9999999999 file.txt")

	// Argument-injection-shaped inputs.
	f.Add("du -- -file.txt")
	f.Add("du --")
	f.Add("du --no-such-flag")
	f.Add("du -????")
	f.Add("du file1 file2 file3 file4 file5")

	// Empty / whitespace.
	f.Add("du")
	f.Add("du ''")
	f.Add("du '   '")

	// Source C — adopted from existing test scenarios.
	f.Add("du -b a.txt b.txt")
	f.Add("du -c -b a.txt b.txt")
	f.Add("du -0 -b a.txt b.txt")
	f.Add("du -d 0 -b top")
	f.Add("du -d 1 -b top")
	f.Add("du -s -b top")
	f.Add("du -a --apparent-size top")

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, script string) {
		if t.Context().Err() != nil {
			return
		}
		if len(script) > 1<<14 {
			return // avoid pathological scripts
		}
		// Skip non-UTF-8 strings: the shell parser rejects them with a
		// parse error before du is ever invoked, which is not a useful
		// signal here.
		if !utf8.ValidString(script) {
			return
		}
		// Restrict the fuzz target to scripts that actually invoke du. The
		// mutator can otherwise produce inputs like "0" that the shell
		// treats as a command-not-found (exit 127), which is not what we
		// are testing.
		if !strings.HasPrefix(script, "du ") && script != "du" {
			return
		}
		// Filter inputs containing shell metacharacters that change the
		// command structure (`&` background, `;` chain, `|` pipe, `<`/`>`
		// redirect, `$` expansion, `` ` `` substitution, `(` subshell,
		// `\n`/`\r` multi-line). Also filter glob metacharacters
		// (`*`, `?`, `[`) because the shell's glob→regex translator can
		// panic on certain multi-byte sequences (a known shell-side bug,
		// not a du bug). The fuzzer is testing du's flag-parsing surface,
		// not the shell's job-control / pipeline / multi-line / glob
		// semantics — those have their own tests.
		if strings.ContainsAny(script, "&;|<>$`(){}\\\n\r*?[") {
			return
		}
		// Filter inputs that would cause shell parse errors. A naive
		// per-quote count is insufficient — quotes nest across single/
		// double contexts (e.g. `"'"'` is balanced byte-for-byte but
		// the trailing `'` opens an unclosed single-quoted string).
		// Pre-parse with the actual parser and drop scripts it rejects.
		if _, err := syntax.NewParser().Parse(strings.NewReader(script), ""); err != nil {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()
		// Pre-create the files referenced by the seed corpus so the
		// happy-path scripts have something to operate on. Also build a
		// 'top' directory used by recursive seeds.
		for _, n := range []string{"file.txt", "a.txt", "b.txt", "file1", "file2", "file3", "file4", "file5"} {
			_ = os.WriteFile(filepath.Join(dir, n), []byte("data"), 0o644)
		}
		_ = os.MkdirAll(filepath.Join(dir, "top", "sub"), 0o755)
		_ = os.WriteFile(filepath.Join(dir, "top", "a.txt"), []byte("xy"), 0o644)
		_ = os.WriteFile(filepath.Join(dir, "top", "sub", "inner.txt"), []byte("zzz"), 0o644)

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		_, _, code := cmdRunCtxFuzz(ctx, t, script, dir)
		if t.Context().Err() != nil {
			return
		}
		// Acceptable exit codes:
		//   0   du success
		//   1   du runtime error (rejected flag, missing file, etc.)
		//   2   shell parse/syntax error (e.g. unsupported ~user expansion)
		//   127 command-not-found from shell expansion oddities
		// Anything else (a panic, SIGSEGV, OOM kill, etc.) is a real bug.
		//
		// NOTE: 127 is broad — it intentionally accepts shell-side issues
		// (e.g. ~user expansion, glob translator quirks) that surface as
		// command-not-found. If the fuzzer stops producing 127 exits over
		// many runs (because the shell tightens those paths), revisit
		// whether 127 should still be in the allowlist; at that point a
		// real du regression producing 127 would otherwise be masked.
		if code != 0 && code != 1 && code != 2 && code != 127 {
			t.Errorf("du unexpected exit code %d for script %q", code, script)
		}
	})
}

// FuzzDuTreeShape fuzzes du's traversal logic by generating directory
// trees of various shapes and running `du` over them.
func FuzzDuTreeShape(f *testing.F) {
	// Each seed encodes a tree shape: a comma-separated list of
	// "<depth>:<name>:<bytes>" tuples. depth 0 = top-level operand.
	f.Add("0:a:5,0:b:10") // two siblings
	f.Add("0:a:5,1:a/sub:0,2:a/sub/x:7")
	f.Add("") // empty (creates only the root)
	f.Add("0:big:1024")
	f.Add("0:zero:0")
	f.Add("0:dir:0,1:dir/file:1024")
	f.Add("0:a:0,1:a/b:0,2:a/b/c:0,3:a/b/c/d:0") // deep chain
	// Large sibling fan-out.
	wide := make([]string, 50)
	for i := range wide {
		wide[i] = fmt.Sprintf("0:f%d:1", i)
	}
	f.Add(strings.Join(wide, ","))

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, spec string) {
		if t.Context().Err() != nil {
			return
		}
		if len(spec) > 1<<13 {
			return
		}
		// Skip inputs the shell parser would reject with a parse error.
		if !utf8.ValidString(spec) {
			return
		}

		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		// Materialise the spec.
		for _, tok := range strings.Split(spec, ",") {
			parts := strings.SplitN(tok, ":", 3)
			if len(parts) != 3 {
				continue
			}
			name := parts[1]
			if name == "" {
				continue
			}
			// Sanitise: reject any path that escapes the temp dir.
			if strings.Contains(name, "..") || strings.HasPrefix(name, "/") {
				continue
			}
			full := filepath.Join(dir, filepath.FromSlash(name))
			parent := filepath.Dir(full)
			_ = os.MkdirAll(parent, 0o755)
			var sz int64
			_, _ = fmt.Sscanf(parts[2], "%d", &sz)
			if sz < 0 || sz > 1<<20 {
				continue
			}
			if sz == 0 {
				_ = os.MkdirAll(full, 0o755)
				continue
			}
			_ = os.WriteFile(full, make([]byte, sz), 0o644)
		}

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		// Run several flag combinations on the same tree to exercise the
		// emit/accumulate paths.
		for _, cmd := range []string{
			"du -b .",
			"du -a -b .",
			"du -s -b .",
			"du -c -b .",
			"du -d 1 -b .",
			"du --apparent-size -h .",
		} {
			_, _, code := cmdRunCtxFuzz(ctx, t, cmd, dir)
			if t.Context().Err() != nil {
				return
			}
			if code != 0 && code != 1 {
				t.Errorf("%q on spec %q unexpected exit code %d", cmd, spec, code)
			}
		}
	})
}

// FuzzDuPath fuzzes the path-handling code of du with arbitrary string
// operands. The corpus exercises path traversal, special characters,
// long names, and binary content in filenames.
func FuzzDuPath(f *testing.F) {
	// Source A — implementation path-handling edges.
	f.Add("file.txt")
	f.Add(".")
	f.Add("..")
	f.Add("./file.txt")
	f.Add("../..")
	f.Add("./././file.txt")
	f.Add("a/b/c/d")
	f.Add("a//b//c")
	f.Add("/absolute/path")
	f.Add("a/.")
	f.Add("a/..")
	// Pathological characters.
	f.Add("file with space.txt")
	f.Add("file\twith\ttabs")
	f.Add("file\nwith\nnewlines")
	f.Add("café.txt")
	f.Add("日本語.txt")
	f.Add("\x00null")
	f.Add(strings.Repeat("a", 200))
	// Path traversal style.
	f.Add("../../../etc/passwd")
	f.Add("..//.././../")

	baseDir := f.TempDir()
	var counter atomic.Int64

	f.Fuzz(func(t *testing.T, path string) {
		if t.Context().Err() != nil {
			return
		}
		if len(path) > 1<<12 {
			return
		}
		// Skip inputs the shell parser would reject with a parse error.
		if !utf8.ValidString(path) {
			return
		}
		// NUL bytes can't appear in a real path; skip.
		if strings.ContainsRune(path, 0) {
			return
		}
		// Skip paths containing characters the shell quoting can't safely
		// round-trip (control characters in C0/C1, isolated CR/LF). The
		// fuzz target here exercises du's path handling, not the shell's
		// quoting rules.
		for _, r := range path {
			if r != '\t' && unicode.IsControl(r) {
				return
			}
		}
		// Don't let the fuzzer escape the temp dir; we test absolute paths
		// separately via the seed corpus. For arbitrary fuzz inputs, just
		// confirm du doesn't crash on the access-denied path.
		dir, cleanup := testutil.FuzzIterDir(t, baseDir, &counter)
		defer cleanup()

		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		// Quote the path so shell-special characters survive parsing. Any
		// single quotes inside the path are escaped using POSIX '\''.
		quoted := "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
		_, _, code := cmdRunCtxFuzz(ctx, t, "du -b "+quoted, dir)
		if t.Context().Err() != nil {
			return
		}
		// Accept du success (0), du runtime error (1), shell parse error
		// (2 — when the path triggers an unsupported expansion), and
		// command-not-found (127). Anything else indicates a panic,
		// SIGSEGV, or other catastrophic failure.
		if code != 0 && code != 1 && code != 2 && code != 127 {
			t.Errorf("du unexpected exit code %d for path %q", code, path)
		}
	})
}
