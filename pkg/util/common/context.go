// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package common provides various helper functions
package common

import (
	"context"
	"sync"
)

var (
	// MainCtx is the main agent context passed to components
	mainCtx context.Context

	// MainCtxCancel cancels the main agent context
	mainCtxCancel context.CancelFunc

	once sync.Once
)

// GetMainCtxCancel returns the shared main context and its cancellation function,
// initialized on the first call. It does not register signal handlers; callers
// control when the context is canceled.
func GetMainCtxCancel() (context.Context, context.CancelFunc) {
	once.Do(func() {
		mainCtx, mainCtxCancel = context.WithCancel(context.Background())
	})

	return mainCtx, mainCtxCancel
}
