// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package model

import "maps"

var (
	overrideVars  = make(map[string]interface{})
	overrideFuncs = make([]func(Config), 0)
)

func init() {
	AddOverrideFunc(applyOverrideVars)
}

// AddOverrideFunc allows to add a custom logic to override configuration.
// This method must be called before Load() to be effective.
func AddOverrideFunc(f func(Config)) {
	overrideFuncs = append(overrideFuncs, f)
}

// AddOverride provides an externally accessible method for
// overriding config variables.
// This method must be called before Load() to be effective.
func AddOverride(name string, value interface{}) {
	overrideVars[name] = value
}

// AddOverrides provides an externally accessible method for
// overriding config variables.
// This method must be called before Load() to be effective.
func AddOverrides(vars map[string]interface{}) {
	maps.Copy(overrideVars, vars)
}

// ApplyOverrideFuncs calls overrideFuncs
func ApplyOverrideFuncs(config Config) {
	for _, f := range overrideFuncs {
		f(config)
	}
}

func applyOverrideVars(config Config) {
	for k, v := range overrideVars {
		if config.IsKnown(k) {
			config.Set(k, v, SourceAgentRuntime)
		}
	}
}
