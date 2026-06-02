// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestResolveConfPathExplicit verifies that an explicitly provided --config
// path is returned unchanged, even if the file does not exist. This preserves
// the existing behavior of erroring loudly when the user asks for a specific
// file that is missing.
func TestResolveConfPathExplicit(t *testing.T) {
	got := resolveConfPath("/this/path/does/not/exist/datadog.yaml")
	assert.Equal(t, "/this/path/does/not/exist/datadog.yaml", got)
}

// TestResolveConfPathDefaultMissing verifies that when --config is not passed
// and the platform default file does not exist, resolveConfPath returns an
// empty string. comp/core/config interprets the empty string as "no explicit
// config file" and tolerates ErrConfigFileNotFound, allowing the agent to
// start with environment-variable configuration only.
func TestResolveConfPathDefaultMissing(t *testing.T) {
	// Override defaultConfigPath to point at a path inside a temp dir that
	// is never created. resolveConfPath observes the absent file via
	// os.Stat and returns the empty string regardless of whether the host
	// has /etc/datadog-agent/datadog.yaml installed.
	origDefault := defaultConfigPath
	defaultConfigPath = filepath.Join(t.TempDir(), "datadog.yaml")
	t.Cleanup(func() { defaultConfigPath = origDefault })

	assert.Equal(t, "", resolveConfPath(""))
}

// TestResolveConfPathDefaultPresent verifies that when --config is not passed
// and a file exists at the default path, resolveConfPath returns the default
// path. This preserves the historical behavior for normal package
// installations (deb/rpm/MSI) that drop a datadog.yaml at the default path.
func TestResolveConfPathDefaultPresent(t *testing.T) {
	// Override defaultConfigPath for the duration of the test by pointing
	// at a temp file. We can't change the package-level var here without
	// race-condition risk, so instead verify the logic by simulating it
	// directly: confirm Stat-then-return semantics for an existing file.
	tmp := filepath.Join(t.TempDir(), "datadog.yaml")
	if err := os.WriteFile(tmp, []byte("api_key: test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	origDefault := defaultConfigPath
	defaultConfigPath = tmp
	t.Cleanup(func() { defaultConfigPath = origDefault })

	assert.Equal(t, tmp, resolveConfPath(""))
}
