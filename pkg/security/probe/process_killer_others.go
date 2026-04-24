// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

// Package probe holds probe related files
package probe

// References to types/fields only used in process_killer.go (linux || windows),
// kept here so the linter doesn't report them as unused on other platforms.

type killContext struct{}

var (
	_ killContext
	_ = KillActionReport{}.pendingKills
)
