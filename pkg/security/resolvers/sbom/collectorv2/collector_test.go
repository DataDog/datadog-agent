// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package collectorv2

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFingerprint(t *testing.T) {
	dir := t.TempDir()
	status := filepath.Join(dir, "var/lib/dpkg/status")
	require.NoError(t, os.MkdirAll(filepath.Dir(status), 0o755))
	require.NoError(t, os.WriteFile(status, []byte("Package: bash\n"), 0o644))
	scanner := NewOSScanner()

	base := scanner.Fingerprint(dir)
	assert.NotEmpty(t, base)
	assert.Equal(t, base, scanner.Fingerprint(dir), "the fingerprint of an unchanged root changed")

	require.NoError(t, os.WriteFile(status, []byte("Package: bash\n\nPackage: curl\n"), 0o644))
	installed := scanner.Fingerprint(dir)
	assert.NotEqual(t, base, installed, "an install kept the fingerprint")

	require.NoError(t, os.Chtimes(status, time.Time{}, time.Now().Add(time.Hour)))
	assert.NotEqual(t, installed, scanner.Fingerprint(dir), "a rewrite of the same size kept the fingerprint")

	assert.Empty(t, scanner.Fingerprint(filepath.Join(dir, "missing")))
}
