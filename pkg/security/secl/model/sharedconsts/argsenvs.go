// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sharedconsts holds model related shared constants
package sharedconsts

const (
	// MaxArgEnvSize maximum size of one argument or environment variable
	// see kernel side definition in custom.h
	MaxArgEnvSize = 356
	// MaxArgsEnvsSize maximum number of args and/or envs
	// see kernel side definition in custom.h
	MaxArgsEnvsSize = 356
)
