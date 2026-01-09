// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaultpaths

import "os"

// defaultCommonRoot is the default path used when DD_COMMON_ROOT is set but empty
const defaultCommonRoot = "/opt/datadog-agent"

// commonRoot holds the common root path for the application package model.
// When set, all path getters will return paths relative to this root.
// This is set early in agent startup via SetCommonRoot() or automatically
// from the DD_COMMON_ROOT environment variable during init().
var commonRoot string

func init() {
	// Check DD_COMMON_ROOT environment variable early so that config defaults
	// are correct when BindEnvAndSetDefault is called during config/setup init().
	if envVal, found := os.LookupEnv("DD_COMMON_ROOT"); found {
		if envVal == "" {
			commonRoot = defaultCommonRoot
		} else {
			commonRoot = envVal
		}
	}
}

// SetCommonRoot sets the common root path for the application package model.
// This should be called early in agent startup, before any path getters are used.
// When set, paths like /etc/datadog-agent become {root}/etc, /var/log/datadog
// becomes {root}/logs, etc.
func SetCommonRoot(root string) {
	commonRoot = root
}

// GetCommonRoot returns the currently configured common root path.
// Returns empty string if the application package model is not enabled.
func GetCommonRoot() string {
	return commonRoot
}
