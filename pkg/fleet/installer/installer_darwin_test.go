// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package installer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/env"
	"github.com/DataDog/datadog-agent/pkg/fleet/installer/macpkg"
)

// TestParsePackageRef pins the mapping from the URL a task names to the package and version the
// pool is addressed by. It is the whole reason the macOS flow can decide what to do before it
// downloads anything, so a change here changes which directory the payload lands in.
func TestParsePackageRef(t *testing.T) {
	for _, test := range []struct {
		url     string
		name    string
		version string
	}{
		{"oci://registry.ddbuild.io/agent-package:7.99.1", "datadog-agent", "7.99.1"},
		{"oci://registry.ddbuild.io/datadog-agent:7.99.1", "datadog-agent", "7.99.1"},
		{"oci://registry.ddbuild.io/some/nested/path/agent-package:7.66.0-1", "datadog-agent", "7.66.0-1"},
		{"registry.ddbuild.io/installer-package:7.99.1", "datadog-installer", "7.99.1"},
	} {
		name, version, err := parsePackageRef(test.url)
		require.NoError(t, err, test.url)
		assert.Equal(t, test.name, name, test.url)
		assert.Equal(t, test.version, version, test.url)
	}
}

// TestParsePackageRefRejectsAnUnversionedRef is the load-bearing half. A URL with no version, or
// one addressed by digest, cannot be turned into a pool directory, and guessing would install a
// payload under a name that is not its version -- which every later link move would then trust.
func TestParsePackageRefRejectsAnUnversionedRef(t *testing.T) {
	for _, url := range []string{
		"",
		"oci://registry.ddbuild.io/agent-package",
		"oci://registry.ddbuild.io/agent-package:",
		"oci://registry.ddbuild.io/agent-package@sha256:0000000000000000000000000000000000000000000000000000000000000000",
	} {
		_, _, err := parsePackageRef(url)
		assert.Error(t, err, "%q was accepted", url)
	}
}

type stubRepository struct {
	present bool
	err     error
}

func (s stubRepository) HasVersion(string) (bool, error) { return s.present, s.err }

func completePayload(t *testing.T, root string, version string) {
	t.Helper()
	for _, path := range requiredPayloadPathsForTest() {
		full := filepath.Join(root, version, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))
		require.NoError(t, os.WriteFile(full, []byte("#!/bin/sh\n"), 0755))
	}
}

// requiredPayloadPathsForTest mirrors what a complete payload has to contain. It is spelled out
// here rather than exported from macpkg so that a path added there fails this test loudly instead
// of quietly changing what "complete" means for the reuse decision.
func requiredPayloadPathsForTest() []string {
	return []string{
		filepath.Join("bin", "agent", "agent"),
		filepath.Join("embedded", "bin", "installer"),
		filepath.Join("embedded", "bin", "system-probe"),
		filepath.Join("embedded", "bin", "agent-data-plane"),
	}
}

// TestMaterializeVersionReusesACompletePayload covers open decision 2: a version already in the
// pool that passes the completeness check is reused. The assertion is that nothing is downloaded,
// which is what makes this observable at all -- the downloader would fail against the empty URL,
// so a reuse that did not happen surfaces as an error rather than as a slow test.
func TestMaterializeVersionReusesACompletePayload(t *testing.T) {
	root := t.TempDir()
	completePayload(t, root, "7.99.1")
	i := &installerImpl{}

	err := i.materializeVersion(
		context.Background(),
		macpkg.PayloadCheck{Root: root},
		stubRepository{present: true},
		"datadog-agent", "", "7.99.1",
	)
	assert.NoError(t, err)
}

// TestMaterializeVersionDoesNotReuseAnIncompletePayload is the other side of the same decision. A
// half-installed tree is the state an interrupted install leaves behind, and reusing it would put
// a link on a directory the Agent cannot start from. Reaching the download is the correct
// behaviour here, so the empty URL failing is the evidence.
func TestMaterializeVersionDoesNotReuseAnIncompletePayload(t *testing.T) {
	root := t.TempDir()
	completePayload(t, root, "7.99.1")
	require.NoError(t, os.Remove(filepath.Join(root, "7.99.1", "embedded", "bin", "system-probe")))
	i := &installerImpl{env: &env.Env{}}

	err := i.materializeVersion(
		context.Background(),
		macpkg.PayloadCheck{Root: root},
		stubRepository{present: true},
		"datadog-agent", "", "7.99.1",
	)
	assert.Error(t, err, "an incomplete payload was reused")
}
