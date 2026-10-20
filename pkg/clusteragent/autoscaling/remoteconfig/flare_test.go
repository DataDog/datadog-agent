// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"testing"

	"github.com/stretchr/testify/require"

	flarehelpers "github.com/DataDog/datadog-agent/comp/core/flare/helpers"
)

// TestAddFlareFilesNotConfigured is the customer case: no endpoint, so the flare
// is exactly what it was before.
func TestAddFlareFilesNotConfigured(t *testing.T) {
	fb := flarehelpers.NewFlareBuilderMock(t, false)

	require.NoError(t, AddFlareFiles(fb, newTestConfig(t, "", "")))

	fb.AssertNoFileExists(DatabaseFileName)
}

// TestAddFlareFiles covers the autoscaling client's uptane cache reaching the
// flare; the default client's cache is collected separately.
func TestAddFlareFiles(t *testing.T) {
	runPath := t.TempDir()
	cfg := newTestConfig(t, "https://config.datad0g.com", testAPIKey)
	cfg.SetInTest("run_path", runPath)

	// Build the client so that its bbolt cache exists on disk, as it would on a
	// running Cluster Agent.
	built, err := NewClient(cfg, "test-hostname", "test-cluster", "test-cluster-id", testProducts, NewEndpointStatus())
	require.NoError(t, err)
	built.Close()

	fb := flarehelpers.NewFlareBuilderMock(t, false)
	require.NoError(t, AddFlareFiles(fb, cfg))

	fb.AssertFileExists(DatabaseFileName)
	// The default client's cache is not this package's business.
	fb.AssertNoFileExists("remote-config.db")
}

// TestAddFlareFilesMissingDatabase covers the endpoint being configured while the
// client never started, e.g. autoscaling is disabled. Reporting the error is
// fine; the caller logs it and the rest of the flare must still be produced.
func TestAddFlareFilesMissingDatabase(t *testing.T) {
	cfg := newTestConfig(t, "https://config.datad0g.com", testAPIKey)
	cfg.SetInTest("run_path", t.TempDir())

	fb := flarehelpers.NewFlareBuilderMock(t, false)

	require.Error(t, AddFlareFiles(fb, cfg))
}
