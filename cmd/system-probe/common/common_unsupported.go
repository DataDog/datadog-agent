// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package common

const (
	// DefaultLogFile exported const should have comment (or a comment on this block) or be unexported
	DefaultLogFile = ""
)
