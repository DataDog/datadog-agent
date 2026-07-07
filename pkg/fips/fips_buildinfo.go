// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fips

import (
	"runtime/debug"
	"strings"
)

// BuiltForFIPS reports whether this binary was built as the FIPS flavor.
// It reads the embedded build info to detect requirefips or goexperiment.systemcrypto
// build tags, which is authoritative regardless of how the Go toolchain was invoked.
// This replaces the previous build-tag-constrained approach which was unreliable on
// some deb builds where goexperiment.systemcrypto was not recognized as a build tag
// even though requirefips was set.
func BuiltForFIPS() bool {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return false
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "-tags":
			for _, tag := range strings.Split(s.Value, ",") {
				tag = strings.TrimSpace(tag)
				if tag == "requirefips" || tag == "goexperiment.systemcrypto" {
					return true
				}
			}
		case "GOEXPERIMENT":
			for _, exp := range strings.Split(s.Value, ",") {
				if strings.TrimSpace(exp) == "systemcrypto" {
					return true
				}
			}
		}
	}
	return false
}
