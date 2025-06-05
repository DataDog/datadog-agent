// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package configsyncimpl implements synchronizing the configuration using the core agent config API
package configsyncimpl

import "time"

// Params defines the parameters for the configsync component.
type Params struct {
	// Timeout is the timeout use for each call to the core-agent
	Timeout time.Duration
	// OnInitSync makes configsync synchronize the configuration at initialization and fails init if we can get the
	// configuration from the core agent
	OnInitSync bool
	// OnInitSyncTimeout represents how long configsync should retry to synchronize configuration at init
	OnInitSyncTimeout time.Duration
}

// NewParams creates a new instance of Params
func NewParams(syncTimeout time.Duration, syncOnInit bool, syncOnInitTimeout time.Duration) Params {
	params := Params{
		Timeout:           syncTimeout,
		OnInitSync:        syncOnInit,
		OnInitSyncTimeout: syncOnInitTimeout,
	}
	return params
}

// NewDefaultParams returns the default params for configsync
func NewDefaultParams() Params {
	return Params{}
}
