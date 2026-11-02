// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package daemon

// macOSFleetEnabledKey opts a macOS host into running the installer daemon. Every .dmg install
// loads the daemon's LaunchDaemon, but the daemon only does anything once this is set.
const macOSFleetEnabledKey = "installer.macos_fleet.enabled"

type boolGetter interface {
	GetBool(key string) bool
}

// daemonEnabledOnPlatform reports whether this platform lets the daemon run with the given
// configuration. On macOS it has to be opted into (macOSFleetEnabledKey).
func daemonEnabledOnPlatform(cfg boolGetter) bool {
	return cfg.GetBool(macOSFleetEnabledKey)
}
