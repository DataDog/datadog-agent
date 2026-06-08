// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !sds

//nolint:revive
package sds

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestScanNoSDS validates that, without the `sds` build tag, Scan reports no
// matches.
func TestScanNoSDS(t *testing.T) {
	matches, err := Scan([]byte("contact me at john@example.com please"))
	require.NoError(t, err, "the no-op scan should not fail")
	require.Empty(t, matches, "the no-op scan should report no match")
}
