// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build python

package python

import "testing"

func TestCheckReadyCheckNameTagValue(t *testing.T) {
	t.Run("uses loadedName when present", func(t *testing.T) {
		got := checkReadyCheckNameTagValue("redisdb", "datadog_checks.redisdb")
		if got != "datadog_checks.redisdb" {
			t.Fatalf("expected loadedName to win, got %q", got)
		}
	})

	t.Run("falls back to moduleName when loadedName empty", func(t *testing.T) {
		got := checkReadyCheckNameTagValue("redisdb", "")
		if got != "redisdb" {
			t.Fatalf("expected moduleName fallback, got %q", got)
		}
	})
}
