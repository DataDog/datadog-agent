// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package service

import (
	"os"
	"strings"
)

const (
	GlobalEnvVar     = "DD_PROCMGR_ENABLED"
	GlobalMarkerPath = "/etc/datadog-agent/.procmgr-enabled"
)

func procmgrEnabled() bool {
	return gateOpen(GlobalEnvVar, GlobalMarkerPath)
}

func EnvTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func gateOpen(envVar, markerPath string) bool {
	v, set := os.LookupEnv(envVar)
	if set {
		return EnvTruthy(v)
	}
	_, err := os.Stat(markerPath)
	return err == nil
}
