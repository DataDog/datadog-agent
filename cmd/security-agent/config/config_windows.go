// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package config implements config
package config

import (
	"github.com/DataDog/datadog-agent/pkg/util/winutil"
)

const (

	// ServiceName is the service name used for the system-probe
	ServiceName = "datadog-security-agent"
)

var (
	// DefaultConfigDir represents the base directory where the configuration
	// is located.  By default it is as specified.  However, windows allows
	// alternate install location, so the init function will figure out what
	// the actual value is, if it's not the default.  It is called
	// DefaultConfigDir because that's what the shared components expect it
	// to be called even though it's not necessarily the default.
	DefaultConfigDir = "c:\\programdata\\datadog\\"
)

func init() {
	pd, err := winutil.GetProgramDataDir()
	if err == nil {
		DefaultConfigDir = pd
	}
}
