// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package vdi

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDCVCatalogContainsAllCounters(t *testing.T) {
	require.Len(t, dcvObjects, 6)
	total := 0
	for _, object := range dcvObjects {
		total += len(object.counters)
	}
	require.Equal(t, 65, total)
}
