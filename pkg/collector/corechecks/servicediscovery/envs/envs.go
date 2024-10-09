// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package envs provides target environment variables of interest.
package envs

import (
	"fmt"
)

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

// GetExpectedEnvs - return list of expected env. variables for testing.
func GetExpectedEnvs() ([]string, map[string]string) {
	var expectedEnvs []string
	var expectedMap = make(map[string]string)

	for env := range targets {
		expectedEnvs = append(expectedEnvs, fmt.Sprintf("%s=true", env))
		expectedMap[env] = "true"
	}
	return expectedEnvs, expectedMap
}

// EnvironmentVariables - collected of targeted environment variables.
type EnvironmentVariables struct {
	vars map[string]string
}

// NewEnvironmentVariables returns a new [EnvironmentVariables] to collect env. variables.
func NewEnvironmentVariables(vars map[string]string) EnvironmentVariables {
	return EnvironmentVariables{
		vars: vars,
	}
}

// GetVars returns the collected environment variables
func (ev *EnvironmentVariables) GetVars() map[string]string {
	return ev.vars
}

// Get returns an environment variable if it is present in the collection
func (ev *EnvironmentVariables) Get(name string) (string, bool) {
	if _, ok := targets[name]; !ok {
		return "", false
	}

	val, ok := ev.vars[name]
	return val, ok
}

// Set saves the environment variable if it is targeted.
// returns true if env variable matches the target
func (ev *EnvironmentVariables) Set(name string, val string) bool {
	if _, ok := targets[name]; !ok {
		return false
	}
	if ev.vars == nil {
		ev.vars = make(map[string]string)
	}
	ev.vars[name] = val

	return false
}
