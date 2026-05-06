// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package slog

import (
	"testing"
	"testing/synctest"
)

func TestDefaultDoesNotSpawnGoroutines(t *testing.T) {
	// synctest.Test panics with a deadlock if any goroutine started inside the
	// bubble remains durably blocked when the function returns. If Default()
	// used handlers.NewAsync, its background goroutine would block on
	// cond.Wait() and this call would panic.
	synctest.Test(t, func(_ *testing.T) {
		Default().Info("test message")
	})
}
