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
		URL:     "oci://install.datad0g.com/com.datadoghq.authoredscripts.helm.addrepo-package:0.0.1-1",
		SHA256:  "cd4ef4c7ef5d394a407d05a1d60f6efac64aa3d507124f55f4385e1b5c6e5bfe",
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
