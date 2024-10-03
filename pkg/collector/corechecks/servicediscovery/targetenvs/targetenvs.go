// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package targetenvs provides functionality to extract target environment variables of interest.
package targetenvs

import (
	"fmt"
	"strings"
)

const (
	// EnvDdService - environment variable DD_SERVICE of the process
	EnvDdService = "DD_SERVICE"
	// EnvDdTags - environment variable DD_TAGS of the process
	EnvDdTags = "DD_TAGS"
	// EnvDiscoveryEnabled - environment variable DD_DISCOVERY_ENABLED of the process
	EnvDiscoveryEnabled = "DD_DISCOVERY_ENABLED"
	// EnvInjectionEnabled - environment variable DD_INJECTION_ENABLED of the process
	EnvInjectionEnabled = "DD_INJECTION_ENABLED"
	// EnvPwd - environment variable of the process to discover working directory
	EnvPwd = "PWD"
	// EnvDotNetDetector - environment variable of the process used for .net detection
	EnvDotNetDetector = "CORECLR_ENABLE_PROFILING"
	// EnvGunicornCmdArgs - environment variable of the Green Unicorn process
	EnvGunicornCmdArgs = "GUNICORN_CMD_ARGS"
	// EnvWsgiApp - environment variable of the Web Server Gateway Interface application process
	EnvWsgiApp = "WSGI_APP"
	// EnvSpringApplicationName - environment variable SPRING_APPLICATION_NAME of the process
	EnvSpringApplicationName = "SPRING_APPLICATION_NAME"
	// EnvJavaDetectorCatalinaOpts - environment variable CATALINA_OPTS of the process
	EnvJavaDetectorCatalinaOpts = "CATALINA_OPTS"
)

const (
	envJavaToolOps           = "JAVA_TOOL_OPTIONS"
	envUnderscoreJavaOptions = "_JAVA_OPTIONS"
	envJdkJavaOptions        = "JDK_JAVA_OPTIONS"
	envJavaOptions           = "JAVA_OPTIONS"
	envJavaJdpaOpts          = "JDPA_OPTS"
)

// envsJavaDetector list of environment variables used for Java detection
// these environment variables pass options to the JVM
var envsJavaDetector = []string{
	envJavaToolOps,
	envUnderscoreJavaOptions,
	envJdkJavaOptions,
	// I'm pretty sure these won't be necessary, as they should be parsed before the JVM sees them
	// but there's no harm in including them
	envJavaOptions,
	envJavaJdpaOpts,
	EnvJavaDetectorCatalinaOpts,
}

// targetsMap list of environment variables of interest, uses computed hash to improve performance.
var targetsMap = map[uint64]string{
	hashBytes([]byte(EnvDdService)):                EnvDdService,
	hashBytes([]byte(EnvDdTags)):                   EnvDdTags,
	hashBytes([]byte(EnvDiscoveryEnabled)):         EnvDiscoveryEnabled,
	hashBytes([]byte(EnvInjectionEnabled)):         EnvInjectionEnabled,
	hashBytes([]byte(EnvPwd)):                      EnvPwd,
	hashBytes([]byte(EnvDotNetDetector)):           EnvDotNetDetector,
	hashBytes([]byte(EnvGunicornCmdArgs)):          EnvGunicornCmdArgs,
	hashBytes([]byte(EnvWsgiApp)):                  EnvWsgiApp,
	hashBytes([]byte(EnvSpringApplicationName)):    EnvSpringApplicationName,
	hashBytes([]byte(envJavaToolOps)):              envJavaToolOps,
	hashBytes([]byte(envUnderscoreJavaOptions)):    envUnderscoreJavaOptions,
	hashBytes([]byte(envJdkJavaOptions)):           envJdkJavaOptions,
	hashBytes([]byte(envJavaOptions)):              envJavaOptions,
	hashBytes([]byte(EnvJavaDetectorCatalinaOpts)): EnvJavaDetectorCatalinaOpts,
	hashBytes([]byte(envJavaJdpaOpts)):             envJavaJdpaOpts,
}

// find - looks for a variable in the environment variable map
func find(envs map[string]string, env string) (string, bool) {
	val, ok := envs[env]
	if !ok {
		return "", false
	}
	return val, len(val) > 0
}

// GetExpectedEnvs - return list of expected env. variables for testing.
func GetExpectedEnvs() ([]string, map[string]string) {
	var expectedEnvs []string
	var expectedMap = make(map[string]string)
	for _, env := range targetsMap {
		expectedEnvs = append(expectedEnvs, fmt.Sprintf("%s=true", env))
		expectedMap[env] = "true"
	}
	return expectedEnvs, expectedMap
}

// ServiceNameInjectionEnabled - returns true if service name injection is enabled.
func ServiceNameInjectionEnabled(envs map[string]string) bool {
	val, ok := find(envs, EnvInjectionEnabled)
	if ok {
		parts := strings.Split(val, ",")
		for _, p := range parts {
			if p == "service_name" {
				return true
			}
		}
	}
	return false
}

// ChooseServiceName - searches and returns the service name in tracer env variables (DD_SERVICE, DD_TAGS).
func ChooseServiceName(envs map[string]string) (string, bool) {
	val, ok := find(envs, EnvDdService)
	if ok {
		return val, true
	}
	val, ok = find(envs, EnvDdTags)
	if ok && strings.Contains(val, "service:") {
		parts := strings.Split(val, ",")
		for _, p := range parts {
			if strings.HasPrefix(p, "service:") {
				return strings.TrimPrefix(p, "service:"), true
			}
		}
	}

	return "", false
}

// TracerInjectionEnabled - returns true if it finds a tracer injection environment variable.
func TracerInjectionEnabled(envs map[string]string) bool {
	val, ok := find(envs, EnvInjectionEnabled)
	if ok {
		parts := strings.Split(val, ",")
		for _, p := range parts {
			if p == "tracer" {
				return true
			}
		}
	}
	return false
}

// WorkingDir - returns the current working directory retrieved from the PWD environment variable.
func WorkingDir(envs map[string]string) (string, bool) {
	return find(envs, "PWD")
}

// DotNetEnabled - returns true if it finds environment variables that enable .NET profiling.
func DotNetEnabled(envs map[string]string) bool {
	val, ok := find(envs, EnvDotNetDetector)
	if ok {
		return val == "1"
	}
	return false
}

// FindJava - returns true if it finds environment variables associated with the Java agent.
func FindJava(envs map[string]string) bool {
	for _, name := range envsJavaDetector {
		if val, ok := envs[name]; ok {
			if strings.Contains(val, "-javaagent:") && strings.Contains(val, "dd-java-agent.jar") {
				return true
			}
		}
	}
	return false
}

// FindGunicornCmdArgs - looks up and returns the environment variable Green Unicorn (Python's HTTP server).
func FindGunicornCmdArgs(envs map[string]string) (string, bool) {
	return find(envs, EnvGunicornCmdArgs)
}

// FindWsgiApp - looks up and returns the env variable of the Web Server Gateway Interface application.
func FindWsgiApp(envs map[string]string) (string, bool) {
	return find(envs, EnvWsgiApp)
}
