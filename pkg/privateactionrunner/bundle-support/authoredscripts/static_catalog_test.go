// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package authoredscripts

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStaticCatalogLookup(t *testing.T) {
	catalog := NewStaticCatalog()

	descriptor, err := catalog.Lookup("com.datadoghq.authoredscripts.helm.addRepo")

	require.NoError(t, err)
	assert.Equal(t, Descriptor{
		Package: "com.datadoghq.authoredscripts.helm.addRepo",
		Version: "0.0.1",
		URL:     "oci://registry.ddbuild.io/dd-authored-scripts/dd-par-scripts-helm-add-repo@sha256:ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c",
		SHA256:  "ea7829a6ebdaa464eb4fbfff4c72e6e63176df58a430a4b0b8dfb66f0e57149c",
	}, descriptor)
}

func TestStaticCatalogLookupUnknownPackage(t *testing.T) {
	catalog := NewStaticCatalog()

	descriptor, err := catalog.Lookup("com.datadoghq.authoredscripts.unknown")

	require.ErrorIs(t, err, ErrPackageNotConfigured)
	assert.Empty(t, descriptor)
}

func TestNilStaticCatalogLookup(t *testing.T) {
	var catalog *StaticCatalog

	_, err := catalog.Lookup("com.datadoghq.authoredscripts.helm.addRepo")

	require.ErrorIs(t, err, ErrPackageNotConfigured)
}
