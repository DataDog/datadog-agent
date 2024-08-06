// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package configsyncimpl implements synchronizing the configuration using the core agent config API
package configsyncimpl

import "time"

// Params defines the parameters for the configsync component.
type Params struct {
	SyncTO     time.Duration
	SyncOnInit bool
}

// NewParams creates a new instance of Params
func NewParams(to time.Duration, sync bool) Params {
	params := Params{
		SyncTO:     to,
		SyncOnInit: sync,
	}
	return params
}
