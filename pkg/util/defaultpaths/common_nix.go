// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build netbsd || openbsd || solaris || dragonfly || linux || aix || darwin || freebsd

package defaultpaths

import (
	"os"
)

// commonRoot holds the common root path for the application package model.
// When set, all path getters will return paths relative to this root.
// This is set automatically from the DD_COMMON_ROOT environment variable during init().
// nolint is needed because this is only implemented for linux right now
var commonRoot string //nolint:unused

func init() {
	setCommonRootFromEnv()
}

func setCommonRootFromEnv() {
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
