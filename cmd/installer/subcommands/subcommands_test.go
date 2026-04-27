// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package subcommands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/installer/command"
)

// TestWithRootRunsFxconfigBootstrap is a regression test for a Cobra
// gotcha: when a child command sets PersistentPreRunE, the root's
// PersistentPreRun is silently skipped (cobra.EnableTraverseRunHooks is
// false by default). Earlier the fxconfig bootstrap was wired on the
// root only — so installer subcommands ran without translating
// datadog.yaml into DD_* env vars, and DDOT post-install hooks read
// empty api_key/site, generating broken otel-config.yaml. This test
// asserts the bootstrap fires for a command produced by withRoot.
func TestWithRootRunsFxconfigBootstrap(t *testing.T) {
	prev, set := os.LookupEnv("DD_API_KEY")
	t.Cleanup(func() {
		if set {
			os.Setenv("DD_API_KEY", prev)
		} else {
			os.Unsetenv("DD_API_KEY")
		}
	})
	os.Unsetenv("DD_API_KEY")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "datadog.yaml"),
		[]byte("api_key: from-yaml\n"), 0o644,
	))

	// Stub leaf command — withRoot will install a PersistentPreRunE on it.
	stub := &cobra.Command{
		Use:  "stub",
		RunE: func(*cobra.Command, []string) error { return nil },
	}

	factory := func(_ *command.GlobalParams) []*cobra.Command {
		return []*cobra.Command{stub}
	}
	wrapped := withRoot(factory)

	global := &command.GlobalParams{
		ConfFilePath: dir,
		AllowNoRoot:  true, // skip the root-required check in test
	}
	cmds := wrapped(global)
	require.Len(t, cmds, 1)
	require.NotNil(t, cmds[0].PersistentPreRunE,
		"withRoot must install PersistentPreRunE on each child")

	// Invoking the wrapped PreRunE should run fxconfig.LoadAndExportEnv
	// (via the closure), which translates yaml api_key → DD_API_KEY.
	require.NoError(t, cmds[0].PersistentPreRunE(cmds[0], nil))
	assert.Equal(t, "from-yaml", os.Getenv("DD_API_KEY"),
		"fxconfig bootstrap must run inside withRoot — see test doc for the cobra gotcha it guards against")
}
