// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !sds

package sds

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestScannerNoSDS mirrors the sds-tagged tests for the build without the
// `sds` tag: NewScanner returns the no-op scanner, so nothing ever matches.
func TestScannerNoSDS(t *testing.T) {
	s, err := NewScanner([]RuleDefinition{
		{ID: "email", Regex: `[a-zA-Z0-9]+@[a-zA-Z0-9]+\.[a-zA-Z0-9]+`},
	})
	require.NoError(t, err, "creating the no-op scanner should not fail")
	t.Cleanup(func() { _ = s.Close() })

	matches, err := s.Scan([]byte("contact me at john@example.com please"))
	require.NoError(t, err, "the no-op scan should not fail")
	require.Empty(t, matches, "the no-op scanner should report no match")
}
