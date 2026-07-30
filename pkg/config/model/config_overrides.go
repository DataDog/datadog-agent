// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

var (
	overrideFuncs = make([]func(Config), 0)
)

// AddOverrideFunc allows to add a custom logic to override configuration.
// This method must be called before Load() to be effective.
func AddOverrideFunc(f func(Config)) {
	overrideFuncs = append(overrideFuncs, f)
}

// ApplyOverrideFuncs calls overrideFuncs
func ApplyOverrideFuncs(config Config) {
	for _, f := range overrideFuncs {
		f(config)
	}
}
