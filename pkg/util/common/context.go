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

// GetMainCtxCancel will return the main context and cancel function and populate them
// the main context can only be populated once
func GetMainCtxCancel() (context.Context, context.CancelFunc) {
	once.Do(func() {
		mainCtx, mainCtxCancel = context.WithCancel(context.Background())
	})

	return mainCtx, mainCtxCancel
}
