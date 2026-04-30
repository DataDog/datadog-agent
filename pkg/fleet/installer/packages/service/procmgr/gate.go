// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package procmgr

import (
	"os"
	"strings"
)

// Env and marker paths for installer gates. Env names match the marker basename
// (DD_PROCMGR_ENABLED ↔ .procmgr-enabled, DD_PROCMGR_DDOT_ENABLED ↔ .procmgr-ddot-enabled).
const (
	GlobalEnvVar     = "DD_PROCMGR_ENABLED"
	GlobalMarkerPath = "/etc/datadog-agent/.procmgr-enabled"
	DDOTEnvVar       = "DD_PROCMGR_DDOT_ENABLED"
	DDOTMarkerPath   = "/etc/datadog-agent/.procmgr-ddot-enabled"
)

// GlobalGateOpen reports whether the host-wide procmgr gate is open (same
// semantics as service.GetServiceManagerType switching to ProcmgrType).
func GlobalGateOpen() bool {
	return ProcessGateOpen(GlobalEnvVar, GlobalMarkerPath)
}

// ProcessGateOpen reports whether a procmgr gate is open for one surface
// (global or a specific process). If envVar is set in the environment, its
// value is authoritative (truthy enables; anything else disables). If unset,
// markerPath existence enables.
func ProcessGateOpen(envVar, markerPath string) bool {
	v, set := os.LookupEnv(envVar)
	if set {
		return envTruthy(v)
	}
	_, err := os.Stat(markerPath)
	return err == nil
}

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "t", "yes", "y", "on":
		return true
	default:
		return false
	}
}
