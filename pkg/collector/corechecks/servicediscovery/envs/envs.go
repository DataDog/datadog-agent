// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package envs provides target environment variables of interest.
package envs

// targets is a collection of environment variables of interest.
var targets = map[string]struct{}{
	"PWD":                      {},
	"DD_INJECTION_ENABLED":     {},
	"DD_SERVICE":               {},
	"DD_TAGS":                  {},
	"DD_DISCOVERY_ENABLED":     {},
	"GUNICORN_CMD_ARGS":        {},
	"WSGI_APP":                 {},
	"CORECLR_ENABLE_PROFILING": {},
	"CATALINA_OPTS":            {},
	"JAVA_TOOL_OPTIONS":        {},
	"_JAVA_OPTIONS":            {},
	"JDK_JAVA_OPTIONS":         {},
	"JAVA_OPTIONS":             {},
	"JDPA_OPTS":                {},
	"SPRING_APPLICATION_NAME":  {},
	"SPRING_CONFIG_LOCATIONS":  {},
	"SPRING_CONFIG_NAME":       {},
	"SPRING_PROFILES_ACTIVE":   {},
}

// Variables - collected of targeted environment variables.
type Variables struct {
	vars map[string]string
}

// Get returns an environment variable if it is present in the collection
func (ev *Variables) Get(name string) (string, bool) {
	val, ok := ev.vars[name]
	return val, ok
}

// GetDefault returns an environment variable or provided default.
func (ev *Variables) GetDefault(name, defVal string) string {
	val, ok := ev.Get(name)
	if !ok {
		return defVal
	}

	return val
}

// Set saves the environment variable if it is targeted.
// returns true if env variable matches the target
func (ev *Variables) Set(name, val string) bool {
	if _, ok := targets[name]; !ok {
		return false
	}
	if ev.vars == nil {
		ev.vars = make(map[string]string)
	}
	ev.vars[name] = val

	return true
}
