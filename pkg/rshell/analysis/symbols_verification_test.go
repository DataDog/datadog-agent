// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// The verification tests serve two purposes: they validate that the core
// allowed-symbols checking logic is correct, and they test each per-config
// allowed-symbols configuration to ensure it catches real violations. They work
// by copying the repo into a temp directory, injecting disallowed imports or
// symbols, and asserting the checker detects them. These helpers factor out the
// repetitive plumbing (copying trees, rewriting Go source, locating files) so
// each per-config test file stays focused on the specific violation it tests.

package analysis

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerificationAgentImportNotExempt(t *testing.T) {
	for _, cfg := range []allowedSymbolsConfig{interpCheckConfig(), allowedpathsCheckConfig()} {
		t.Run(cfg.TargetDir, func(t *testing.T) {
			tmp := t.TempDir()
			copyDir(t, filepath.Join(repoRoot(t), cfg.TargetDir), filepath.Join(tmp, cfg.TargetDir))
			writeGoFile(t, filepath.Join(tmp, cfg.TargetDir, "agent_import.go"), cfg.TargetDir,
				[]string{`"github.com/DataDog/datadog-agent/pkg/util/log"`}, "var _ = log.Info\n")

			var errs []string
			cfg.RepoRootOverride = tmp
			cfg.Errors = &errs
			checkAllowedSymbols(t, cfg)
			if !errContains(errs, "github.com/DataDog/datadog-agent/pkg/util/log") {
				t.Fatalf("expected a disallowed Agent import, got: %v", errs)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Shared helpers for verification tests
// ---------------------------------------------------------------------------

// copyDir recursively copies src to dst. Only regular files and directories
// are handled.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// repoRoot returns the repo root by going one level up from the test's working
// directory (analysis/).
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Dir(dir)
}

// errContains returns true if any error in errs contains substr.
func errContains(errs []string, substr string) bool {
	for _, e := range errs {
		if strings.Contains(e, substr) {
			return true
		}
	}
	return false
}

// injectImport reads a Go source file and inserts an import line into
// the first import block. It also optionally appends extra code after the file.
func injectImport(t *testing.T, path, importLine, appendCode string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	content = strings.Replace(content, "import (", "import (\n\t"+importLine, 1)
	if appendCode != "" {
		content += "\n" + appendCode + "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// injectUnlistedSymbol adds a usage of os.Setenv (an unlisted symbol from an
// allowed package) to the file at path, adding an os import if needed.
func injectUnlistedSymbol(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, `"os"`) {
		content = strings.Replace(content, "import (", "import (\n\t\"os\"", 1)
	}
	content += "\nvar _ = os.Setenv\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeGoFile creates a minimal Go source file at path with the given package
// name, imports, and body lines.
func writeGoFile(t *testing.T, path, pkg string, imports []string, body string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("package " + pkg + "\n\n")
	if len(imports) > 0 {
		b.WriteString("import (\n")
		for _, imp := range imports {
			b.WriteString("\t" + imp + "\n")
		}
		b.WriteString(")\n\n")
	}
	b.WriteString(body)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// findFirstSubdirGoFile returns the path to the first non-test .go file inside
// a subdirectory of dir (not at the top level).
func findFirstSubdirGoFile(t *testing.T, dir string) string {
	t.Helper()
	var found string
	var fallback string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		if !strings.HasSuffix(info.Name(), ".go") || strings.HasSuffix(info.Name(), "_test.go") {
			return nil
		}
		if strings.Contains(rel, string(filepath.Separator)) {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), "import (") {
				found = path
				return filepath.SkipAll
			}
			if fallback == "" {
				fallback = path
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found == "" {
		found = fallback
	}
	if found == "" {
		t.Fatalf("no .go file found in subdirectories of %s", dir)
	}
	return found
}

// findFirstFlatGoFile returns the path to the first non-test .go file directly
// in dir (not in subdirectories).
func findFirstFlatGoFile(t *testing.T, dir string) string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".go") && !strings.HasSuffix(e.Name(), "_test.go") {
			return filepath.Join(dir, e.Name())
		}
	}
	t.Fatalf("no .go file found in %s", dir)
	return ""
}
