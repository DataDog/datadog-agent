// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

package yara

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRule = `rule evil_marker : test {
    strings:
        $a = "EVIL_MARKER"
    condition:
        $a
}
`

func writeRuleFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644))
	}
}

// trustCurrentUser makes the rule loader accept files owned by the test's uid, since tests can't
// create root-owned files when they don't run as root
func trustCurrentUser(t *testing.T) {
	t.Helper()
	setTrustedRuleOwner(t, uint32(os.Getuid()))
}

func setTrustedRuleOwner(t *testing.T, uid uint32) {
	t.Helper()
	previous := trustedRuleOwnerUID
	trustedRuleOwnerUID = uid
	t.Cleanup(func() { trustedRuleOwnerUID = previous })
}

// fakeFileInfo is an os.FileInfo with a chosen owner and mode
type fakeFileInfo struct {
	os.FileInfo
	mode os.FileMode
	uid  uint32
}

func (f fakeFileInfo) Mode() os.FileMode { return f.mode }
func (f fakeFileInfo) Sys() any          { return &syscall.Stat_t{Uid: f.uid} }

func TestCheckRulePermissions(t *testing.T) {
	assert.Equal(t, uint32(0), trustedRuleOwnerUID, "only root is trusted by default")

	for name, tc := range map[string]struct {
		info os.FileInfo
		safe bool
	}{
		"root 0644":            {info: fakeFileInfo{mode: 0o644}, safe: true},
		"root 0600":            {info: fakeFileInfo{mode: 0o600}, safe: true},
		"root dir 0755":        {info: fakeFileInfo{mode: os.ModeDir | 0o755}, safe: true},
		"root group writable":  {info: fakeFileInfo{mode: 0o664}},
		"root world writable":  {info: fakeFileInfo{mode: 0o646}},
		"root dir 0777":        {info: fakeFileInfo{mode: os.ModeDir | 0o777}},
		"sticky world dir":     {info: fakeFileInfo{mode: os.ModeDir | os.ModeSticky | 0o777}},
		"non-root owner 0644":  {info: fakeFileInfo{mode: 0o644, uid: 1000}},
		"dd-agent owner 0600":  {info: fakeFileInfo{mode: 0o600, uid: 998}},
		"no owner information": {info: fakeFileInfo{mode: 0o644, uid: 0}.withoutSys()},
	} {
		t.Run(name, func(t *testing.T) {
			err := checkRulePermissions("/etc/datadog-agent/yara.d/r.yar", tc.info)
			if tc.safe {
				assert.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrUnsafeRules)
			}
		})
	}
}

// withoutSys returns a FileInfo whose Sys() is nil
func (f fakeFileInfo) withoutSys() os.FileInfo { return noSysFileInfo{f} }

type noSysFileInfo struct{ fakeFileInfo }

func (noSysFileInfo) Sys() any { return nil }

func TestLoadRuleSourcesOwner(t *testing.T) {
	dir := t.TempDir()
	writeRuleFiles(t, dir, map[string]string{"a.yar": testRule})

	// files owned by another uid than the trusted one are refused
	setTrustedRuleOwner(t, uint32(os.Getuid())+1)
	_, _, err := LoadRuleSources(dir)
	require.ErrorIs(t, err, ErrUnsafeRules)
	assert.Contains(t, err.Error(), dir)

	// with the default trusted uid, root: files created by the test are only accepted when it
	// runs as root
	setTrustedRuleOwner(t, 0)
	_, _, err = LoadRuleSources(dir)
	if os.Geteuid() == 0 {
		assert.NoError(t, err)
	} else {
		assert.ErrorIs(t, err, ErrUnsafeRules)
	}
}

func TestLoadRuleSourcesWritable(t *testing.T) {
	trustCurrentUser(t)

	t.Run("writable dir", func(t *testing.T) {
		for _, mode := range []os.FileMode{0o775, 0o757, 0o777} {
			dir := t.TempDir()
			writeRuleFiles(t, dir, map[string]string{"a.yar": testRule})
			require.NoError(t, os.Chmod(dir, mode))
			_, _, err := LoadRuleSources(dir)
			assert.ErrorIs(t, err, ErrUnsafeRules, mode.String())
		}
	})

	t.Run("writable file", func(t *testing.T) {
		for _, mode := range []os.FileMode{0o664, 0o646, 0o666} {
			dir := t.TempDir()
			writeRuleFiles(t, dir, map[string]string{"a.yar": testRule, "b.yar": testRule})
			path := filepath.Join(dir, "b.yar")
			require.NoError(t, os.Chmod(path, mode))
			_, _, err := LoadRuleSources(dir)
			require.ErrorIs(t, err, ErrUnsafeRules, mode.String())
			assert.Contains(t, err.Error(), path)
		}
	})

	t.Run("symlink to a writable file", func(t *testing.T) {
		dir := t.TempDir()
		elsewhere := t.TempDir()
		writeRuleFiles(t, dir, map[string]string{"a.yar": testRule})
		writeRuleFiles(t, elsewhere, map[string]string{"target.yar": testRule})
		require.NoError(t, os.Chmod(filepath.Join(elsewhere, "target.yar"), 0o666))
		require.NoError(t, os.Symlink(filepath.Join(elsewhere, "target.yar"), filepath.Join(dir, "link.yar")))
		_, _, err := LoadRuleSources(dir)
		assert.ErrorIs(t, err, ErrUnsafeRules)
	})

	t.Run("writable non-rule files are ignored", func(t *testing.T) {
		dir := t.TempDir()
		writeRuleFiles(t, dir, map[string]string{"a.yar": testRule, "notes.txt": "x"})
		require.NoError(t, os.Chmod(filepath.Join(dir, "notes.txt"), 0o666))
		_, _, err := LoadRuleSources(dir)
		assert.NoError(t, err)
	})

	t.Run("not a directory", func(t *testing.T) {
		dir := t.TempDir()
		writeRuleFiles(t, dir, map[string]string{"a.yar": testRule})
		_, _, err := LoadRuleSources(filepath.Join(dir, "a.yar"))
		assert.Error(t, err)
	})
}

type closingScanner struct {
	*MarkerScanner
	closed bool
}

func (s *closingScanner) Close() error {
	s.closed = true
	return nil
}

func TestLoadScannerClose(t *testing.T) {
	trustCurrentUser(t)
	dir := t.TempDir()
	writeRuleFiles(t, dir, map[string]string{"a.yar": testRule})

	inner := &closingScanner{MarkerScanner: NewMarkerScanner("m", "r")}
	scanner, _, err := LoadScanner(dir, func([]RuleSource) (Scanner, error) { return inner, nil })
	require.NoError(t, err)
	closer, ok := scanner.(io.Closer)
	require.True(t, ok)
	require.NoError(t, closer.Close())
	assert.True(t, inner.closed, "Close is forwarded to the engine's scanner")

	// a scanner without Close
	scanner, _, err = LoadScanner(dir, StandInCompiler)
	require.NoError(t, err)
	assert.NoError(t, scanner.(io.Closer).Close())
}

func TestLoadRuleSources(t *testing.T) {
	trustCurrentUser(t)
	dir := t.TempDir()
	writeRuleFiles(t, dir, map[string]string{
		"b.yara":      "rule b { strings: $a = \"B\" condition: $a }",
		"a.yar":       testRule,
		"C.YAR":       "rule c { strings: $a = \"C\" condition: $a }",
		"README.md":   "not a rule",
		"notes.txt":   "rule x { }",
		"backup.yar~": "rule y { }",
	})
	require.NoError(t, os.Mkdir(filepath.Join(dir, "sub.yar"), 0o755))
	writeRuleFiles(t, filepath.Join(dir, "sub.yar"), map[string]string{"nested.yar": testRule})
	require.NoError(t, os.Symlink(filepath.Join(dir, "a.yar"), filepath.Join(dir, "link.yar")))

	sources, version, err := LoadRuleSources(dir)
	require.NoError(t, err)
	names := make([]string, 0, len(sources))
	for _, s := range sources {
		names = append(names, s.Name)
	}
	assert.Equal(t, []string{"C.YAR", "a.yar", "b.yara", "link.yar"}, names, "sorted, rule files only, symlinks followed, no recursion")
	assert.Equal(t, testRule, string(sources[1].Data))
	assert.Equal(t, RulesVersion(sources), version)
	assert.Len(t, version, 64)
}

func TestLoadRuleSourcesErrors(t *testing.T) {
	trustCurrentUser(t)
	_, _, err := LoadRuleSources("")
	assert.ErrorIs(t, err, ErrRulesDirUnset)

	_, _, err = LoadRuleSources(filepath.Join(t.TempDir(), "missing"))
	assert.ErrorIs(t, err, os.ErrNotExist)

	empty := t.TempDir()
	_, _, err = LoadRuleSources(empty)
	assert.ErrorIs(t, err, ErrNoRules)

	writeRuleFiles(t, empty, map[string]string{"readme.txt": "nothing"})
	_, _, err = LoadRuleSources(empty)
	assert.ErrorIs(t, err, ErrNoRules)

	// a dangling symlink is an unreadable rule file
	dangling := t.TempDir()
	require.NoError(t, os.Symlink(filepath.Join(dangling, "gone"), filepath.Join(dangling, "gone.yar")))
	_, _, err = LoadRuleSources(dangling)
	assert.Error(t, err)

	if os.Geteuid() != 0 {
		unreadable := t.TempDir()
		writeRuleFiles(t, unreadable, map[string]string{"r.yar": testRule})
		require.NoError(t, os.Chmod(filepath.Join(unreadable, "r.yar"), 0))
		_, _, err = LoadRuleSources(unreadable)
		assert.ErrorIs(t, err, os.ErrPermission)
	}
}

func TestRulesVersion(t *testing.T) {
	base := []RuleSource{{Name: "a.yar", Data: []byte("A")}, {Name: "b.yar", Data: []byte("B")}}
	v := RulesVersion(base)

	assert.Equal(t, v, RulesVersion(base), "stable")
	assert.Equal(t, v, RulesVersion([]RuleSource{base[1], base[0]}), "order independent")

	for name, other := range map[string][]RuleSource{
		"content changed": {{Name: "a.yar", Data: []byte("A")}, {Name: "b.yar", Data: []byte("C")}},
		"file renamed":    {{Name: "a.yar", Data: []byte("A")}, {Name: "c.yar", Data: []byte("B")}},
		"file removed":    {{Name: "a.yar", Data: []byte("A")}},
		"bytes moved":     {{Name: "a.yarA", Data: nil}, {Name: "b.yar", Data: []byte("B")}},
	} {
		assert.NotEqual(t, v, RulesVersion(other), name)
	}
}

func TestRulesVersionFromDisk(t *testing.T) {
	trustCurrentUser(t)
	dir := t.TempDir()
	writeRuleFiles(t, dir, map[string]string{"a.yar": testRule})
	_, v1, err := LoadRuleSources(dir)
	require.NoError(t, err)
	_, v2, err := LoadRuleSources(dir)
	require.NoError(t, err)
	assert.Equal(t, v1, v2)

	writeRuleFiles(t, dir, map[string]string{"a.yar": testRule + "\n"})
	_, v3, err := LoadRuleSources(dir)
	require.NoError(t, err)
	assert.NotEqual(t, v1, v3)
}

func TestLoadScanner(t *testing.T) {
	trustCurrentUser(t)
	dir := t.TempDir()
	writeRuleFiles(t, dir, map[string]string{"a.yar": testRule})

	scanner, version, err := LoadScanner(dir, StandInCompiler)
	require.NoError(t, err)
	assert.Equal(t, version, scanner.RulesVersion(), "the scanner reports the hash of the rule files")

	matches, err := scanner.Scan(context.Background(), []byte("xx EVIL_MARKER xx"))
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "evil_marker", matches[0].Rule)

	matches, err = scanner.Scan(context.Background(), []byte("clean"))
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestLoadScannerErrors(t *testing.T) {
	trustCurrentUser(t)
	dir := t.TempDir()
	writeRuleFiles(t, dir, map[string]string{"a.yar": testRule})

	compileErr := errors.New("syntax error")
	_, _, err := LoadScanner(dir, func([]RuleSource) (Scanner, error) { return nil, compileErr })
	assert.ErrorIs(t, err, compileErr)

	_, _, err = LoadScanner(dir, func([]RuleSource) (Scanner, error) { return nil, nil })
	assert.Error(t, err)

	_, _, err = LoadScanner(dir, nil)
	assert.Error(t, err)

	_, _, err = LoadScanner("", StandInCompiler)
	assert.ErrorIs(t, err, ErrRulesDirUnset)

	_, _, err = LoadScanner(t.TempDir(), StandInCompiler)
	assert.ErrorIs(t, err, ErrNoRules)

	// the compiler sees every source, sorted
	writeRuleFiles(t, dir, map[string]string{"0.yar": "rule zero { strings: $a = \"0\" condition: $a }"})
	var seen []string
	_, _, err = LoadScanner(dir, func(sources []RuleSource) (Scanner, error) {
		for _, s := range sources {
			seen = append(seen, s.Name)
		}
		return NewMarkerScanner("m", "r"), nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"0.yar", "a.yar"}, seen)
}

func TestStandInCompiler(t *testing.T) {
	scanner, err := StandInCompiler([]RuleSource{
		{Name: "a.yar", Data: []byte(testRule)},
		{Name: "b.yar", Data: []byte(`
import "pe"
private rule helper { strings: $h = "HELPER" condition: $h }
rule second
{
    strings:
        $s = "SECOND"
    condition:
        $s
}`)},
	})
	require.NoError(t, err)

	matches, err := scanner.Scan(context.Background(), []byte("EVIL_MARKER and SECOND"))
	require.NoError(t, err)
	var rules []string
	for _, m := range matches {
		rules = append(rules, m.Rule)
	}
	assert.Equal(t, []string{"evil_marker", "second"}, rules)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = scanner.Scan(ctx, []byte("EVIL_MARKER"))
	assert.ErrorIs(t, err, context.Canceled)

	_, err = StandInCompiler([]RuleSource{{Name: "x.yar", Data: []byte("not a rule")}})
	assert.Error(t, err, "no rule")
	_, err = StandInCompiler([]RuleSource{{Name: "x.yar", Data: []byte("rule empty { condition: true }")}})
	assert.Error(t, err, "no quoted string")
}
