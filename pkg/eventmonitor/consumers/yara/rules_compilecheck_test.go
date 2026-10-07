// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && yara

package yara

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	goyara "github.com/hillu/go-yara/v4"
)

// TestEmbeddedRulesCompileTogether compiles the whole embedded rule set together with the real
// libyara engine, exactly as the production loader does. It is the regression guard that keeps the
// embedded set loadable: the loader compiles every file at once and one bad file disables the whole
// scanner, so this must stay green. Unlike TestEmbeddedRulesCompileIndividually it needs no env var
// and runs in the normal (yara-tag) test variant.
func TestEmbeddedRulesCompileTogether(t *testing.T) {
	sources, err := EmbeddedRuleSources()
	require.NoError(t, err)
	require.NotEmpty(t, sources, "the binary must ship embedded rules")

	scanner, err := DefaultCompiler()(sources)
	require.NoError(t, err, "the whole embedded rule set must compile together")
	require.NotNil(t, scanner)
	if c, ok := scanner.(io.Closer); ok {
		assert.NoError(t, c.Close())
	}
	t.Logf("embedded rule set compiled: %d files", len(sources))
}

// TestLoadScannerWithEmbedded checks that the production loader builds a working scanner from the
// embedded set alone, and that rules in a dir are added on top of it.
func TestLoadScannerWithEmbedded(t *testing.T) {
	// embedded only
	scanner, version, err := LoadScannerWithEmbedded("", DefaultCompiler())
	require.NoError(t, err)
	require.NotNil(t, scanner)
	assert.Len(t, version, 64)
	if c, ok := scanner.(io.Closer); ok {
		t.Cleanup(func() { _ = c.Close() })
	}

	// embedded + a dir added on top: the merged version differs from embedded-only
	trustCurrentUser(t)
	dir := t.TempDir()
	writeRuleFiles(t, dir, map[string]string{"zz_extra.yar": testRule})
	scanner2, version2, err := LoadScannerWithEmbedded(dir, DefaultCompiler())
	require.NoError(t, err)
	require.NotNil(t, scanner2)
	assert.NotEqual(t, version, version2, "adding a dir rule changes the rules version")
	if c, ok := scanner2.(io.Closer); ok {
		t.Cleanup(func() { _ = c.Close() })
	}
}

// TestMergeRuleSources checks the dir-over-embedded precedence on a filename collision.
func TestMergeRuleSources(t *testing.T) {
	base := []RuleSource{{Name: "a.yar", Data: []byte("A")}, {Name: "b.yar", Data: []byte("B")}}
	overrides := []RuleSource{{Name: "b.yar", Data: []byte("B2")}, {Name: "c.yar", Data: []byte("C")}}
	merged := mergeRuleSources(base, overrides)

	got := map[string]string{}
	for _, s := range merged {
		got[s.Name] = string(s.Data)
	}
	assert.Equal(t, map[string]string{"a.yar": "A", "b.yar": "B2", "c.yar": "C"}, got)
	// sorted by name
	assert.Equal(t, []string{"a.yar", "b.yar", "c.yar"}, []string{merged[0].Name, merged[1].Name, merged[2].Name})
}

// TestEmbeddedRulesCompileIndividually compiles every *.yar / *.yara file in the directory named by
// YARA_RULES_DIR individually against the real libyara engine, exactly as compileLibyara does
// (goyara.NewCompiler + AddString, one namespace per file). The files that fail are written to the
// path named by YARA_FAIL_OUT, one "name: error" per line, so the caller can drop them from the
// embedded set. The test is a no-op unless YARA_RULES_DIR is set, so it never runs in normal CI.
//
// This is the vetting step for the embedded rule set (embedded_rules.go): the production loader
// compiles every rule file together and a single bad file disables the whole scanner, so only files
// that compile cleanly may be embedded.
func TestEmbeddedRulesCompileIndividually(t *testing.T) {
	dir := os.Getenv("YARA_RULES_DIR")
	if dir == "" {
		t.Skip("YARA_RULES_DIR not set; this is the embedded-rules vetting helper")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read rules dir: %v", err)
	}

	var failures []string
	var checked int
	for _, entry := range entries {
		if entry.IsDir() || !isRuleFileName(entry.Name()) {
			continue
		}
		checked++
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			failures = append(failures, entry.Name()+": read error: "+err.Error())
			continue
		}
		if err := compileOne(entry.Name(), data); err != nil {
			failures = append(failures, entry.Name()+": "+err.Error())
		}
	}

	sort.Strings(failures)
	t.Logf("checked %d rule files, %d failed to compile", checked, len(failures))

	// Emit each failure to stdout with a stable prefix, so the caller can collect them from the
	// streamed test log even when YARA_FAIL_OUT isn't writable (e.g. a Bazel sandbox).
	for _, f := range failures {
		fmt.Println("YARA_COMPILE_FAIL\t" + strings.ReplaceAll(f, "\n", " "))
	}

	if out := os.Getenv("YARA_FAIL_OUT"); out != "" {
		if err := os.WriteFile(out, []byte(strings.Join(failures, "\n")+"\n"), 0o644); err != nil {
			t.Logf("could not write failure list to %s: %v", out, err)
		} else {
			t.Logf("wrote failure list to %s", out)
		}
	}
}

// compileOne compiles a single rule file's bytes in its own namespace, like compileLibyara, and
// returns the libyara compile error (with line numbers) if it fails.
func compileOne(name string, data []byte) error {
	compiler, err := goyara.NewCompiler()
	if err != nil {
		return err
	}
	defer compiler.Destroy()
	compiler.DisableIncludes()
	if err := compiler.AddString(string(data), name); err != nil {
		return compileError(compiler, err)
	}
	if _, err := compiler.GetRules(); err != nil {
		return err
	}
	return nil
}
