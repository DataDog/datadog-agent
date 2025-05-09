// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// Package sysvinit provides a set of functions to manage sysvinit services
package sysvinit

import (
	"context"
	"os/exec"
)

// Install installs a sys-v init script using update-rc.d
func Install(ctx context.Context) error {
	
}
