// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package commands

import (
	"syscall"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// setInstallerUmask sets umask 0 to override any inherited umask
func setInstallerUmask(span *telemetry.Span) {
	oldmask := syscall.Umask(0)
	span.SetTag("inherited_umask", oldmask)
}
