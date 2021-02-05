// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package status

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNtpWarning(t *testing.T) {
	require.False(t, ntpWarning(1))
	require.False(t, ntpWarning(-1))
	require.True(t, ntpWarning(3601))
	require.True(t, ntpWarning(-601))
}

func TestMkHuman(t *testing.T) {
	f := 1695783.0
	fStr := mkHuman(f)
	if fStr != "1,695,783" {
		t.Errorf("Large number formatting is incorrectly adding commas in agent statuses")
	}
}
