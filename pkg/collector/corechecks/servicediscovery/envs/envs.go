// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package envs provides target environment variables of interest.
package envs

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/servicediscovery/language"
)

// envKind separates environment variables by type, it helps to know when all target variables
// have been found and there is no need to search for variables further.
type envKind uint8

const (
	empty      envKind = 0
	pwdEnv             = 1 << (iota - 1) // system settings env variable detected
	appEnv                               // application system settings env variable detected
	ddInjected                           // agent injection env variable detected
	ddService                            // agent service env variable detected
	ddTags                               // agent tags env variable detected
)

type envLang struct {
	kind envKind
	lang language.Language
}

// targets is a collection of environment variables of interest.
var targets = map[string]envLang{
	"PWD":                      {pwdEnv, language.Unknown},     // fetch it for any application
	"DD_INJECTION_ENABLED":     {ddInjected, language.Unknown}, // fetch it for any application
	"DD_SERVICE":               {ddService, language.Unknown},  // if found, then do not need DD_TAGS
	"DD_TAGS":                  {ddTags, language.Unknown},     // check it for 'service:' value
	"DD_DISCOVERY_ENABLED":     {empty, language.Unknown},      // optional and does not affect service detection
	"GUNICORN_CMD_ARGS":        {appEnv, language.Python},
	"WSGI_APP":                 {appEnv, language.Python},
	"CORECLR_ENABLE_PROFILING": {appEnv, language.DotNet},
	"CATALINA_OPTS":            {appEnv, language.Java},
	"JAVA_TOOL_OPTIONS":        {appEnv, language.Java},
	"_JAVA_OPTIONS":            {appEnv, language.Java},
	"JDK_JAVA_OPTIONS":         {appEnv, language.Java},
	"JAVA_OPTIONS":             {appEnv, language.Java},
	"JDPA_OPTS":                {appEnv, language.Java},
	"SPRING_APPLICATION_NAME":  {appEnv, language.Java},
	"SPRING_CONFIG_LOCATIONS":  {appEnv, language.Java},
	"SPRING_CONFIG_NAME":       {appEnv, language.Java},
	"SPRING_PROFILES_ACTIVE":   {appEnv, language.Java},
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
	lang language.Language // service language
	pile envKind           // accumulates detected kinds of environment variables.
	vars map[string]string
}

// NewEnvironmentVariables returns a new [EnvironmentVariables] to collect env. variables.
func NewEnvironmentVariables(vars map[string]string, lang language.Language) EnvironmentVariables {
	return EnvironmentVariables{
		lang: lang,
		pile: empty,
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
// returns true if the search should be stopped
func (ev *EnvironmentVariables) Set(name string, val string) bool {
	if env, ok := targets[name]; ok {
		if ev.vars == nil {
			ev.vars = make(map[string]string)
		}
		ev.vars[name] = val

		if env.kind == ddTags && strings.Contains(val, "service:") {
			ev.pile |= ddService
		} else {
			ev.pile |= env.kind
		}
		if ev.pile == pwdEnv|ddInjected|ddService|appEnv {
			return true
		}
	}

	return false
}
