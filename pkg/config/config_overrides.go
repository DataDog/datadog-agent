// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package config

var overrideVars = make(map[string]interface{})

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
	for k, v := range vars {
		overrideVars[k] = v
	}
}

func applyOverrideFuncs(config Config) {
	for _, f := range overrideFuncs {
		f(config)
	}
}

func applyOverrideVars(config Config) {
	for k, v := range overrideVars {
		config.Set(k, v)
	}
}
