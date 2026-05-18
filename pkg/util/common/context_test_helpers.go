// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package common

import (
	"sync"
)

// ResetMainCtx resets the global main context to a fresh cancellable context.
func ResetMainCtx() {
	once = sync.Once{}
}
