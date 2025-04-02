// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"
	"slices"

	corev1 "k8s.io/api/core/v1"
)

const (
	// Java config
	javaToolOptionsKey   = "JAVA_TOOL_OPTIONS"
	javaToolOptionsValue = " -javaagent:/datadog-lib/dd-java-agent.jar -XX:OnError=/datadog-lib/java/continuousprofiler/tmp/dd_crash_uploader.sh -XX:ErrorFile=/datadog-lib/java/continuousprofiler/tmp/hs_err_pid_%p.log"

	// Node config
	nodeOptionsKey   = "NODE_OPTIONS"
	nodeOptionsValue = " --require=/datadog-lib/node_modules/dd-trace/init"

	// Python config
	pythonPathKey   = "PYTHONPATH"
	pythonPathValue = "/datadog-lib/"

	// Dotnet config
	dotnetClrEnableProfilingKey   = "CORECLR_ENABLE_PROFILING"
	dotnetClrEnableProfilingValue = "1"

	dotnetClrProfilerIDKey   = "CORECLR_PROFILER"
	dotnetClrProfilerIDValue = "{846F5F1C-F9AE-4B07-969E-05C26BC060D8}"

	dotnetClrProfilerPathKey   = "CORECLR_PROFILER_PATH"
	dotnetClrProfilerPathValue = "/datadog-lib/Datadog.Trace.ClrProfiler.Native.so"

	dotnetTracerHomeKey   = "DD_DOTNET_TRACER_HOME"
	dotnetTracerHomeValue = "/datadog-lib"

	dotnetTracerLogDirectoryKey   = "DD_TRACE_LOG_DIRECTORY"
	dotnetTracerLogDirectoryValue = "/datadog-lib/logs"

	dotnetProfilingLdPreloadKey   = "LD_PRELOAD"
	dotnetProfilingLdPreloadValue = "/datadog-lib/continuousprofiler/Datadog.Linux.ApiWrapper.x64.so"

	// Ruby config
	rubyOptKey   = "RUBYOPT"
	rubyOptValue = " -r/datadog-lib/auto_inject"

	// EnvNames
	instrumentationInstallTypeEnvVarName = "DD_INSTRUMENTATION_INSTALL_TYPE"
	instrumentationInstallTimeEnvVarName = "DD_INSTRUMENTATION_INSTALL_TIME"
	instrumentationInstallIDEnvVarName   = "DD_INSTRUMENTATION_INSTALL_ID"

	// Values for Env variable DD_INSTRUMENTATION_INSTALL_TYPE
	singleStepInstrumentationInstallType   = "k8s_single_step"
	localLibraryInstrumentationInstallType = "k8s_lib_injection"
)

type envVar struct {
	key string

	valFunc   envValFunc
	rawEnvVar *corev1.EnvVar

	isEligibleToInject func(*corev1.Container) bool
	prepend            bool
}

func (e envVar) nextEnvVar(prior corev1.EnvVar, found bool) (corev1.EnvVar, error) {
	if e.rawEnvVar != nil {
		return *e.rawEnvVar, nil
	}

	if !found {
		return corev1.EnvVar{
			Name:  e.key,
			Value: e.valFunc(""),
		}, nil
	}

	if prior.ValueFrom != nil {
		return prior, fmt.Errorf("%q is defined via ValueFrom", e.key)
	}

	prior.Value = e.valFunc(prior.Value)
	return prior, nil
}

// mutateContainer implements containerMutator for envVar.
func (e envVar) mutateContainer(c *corev1.Container) error {
	if e.isEligibleToInject != nil && !e.isEligibleToInject(c) {
		return nil
	}

	index := slices.IndexFunc(c.Env, func(ev corev1.EnvVar) bool {
		return ev.Name == e.key
	})

	var found bool
	var priorEnvVar corev1.EnvVar
	if index >= 0 {
		priorEnvVar = c.Env[index]
		found = true
	}

	nextEnvVar, err := e.nextEnvVar(priorEnvVar, found)
	if err != nil {
		return err
	}

	if found {
		c.Env[index] = nextEnvVar
	} else {
		c.Env = appendOrPrepend(nextEnvVar, c.Env, e.prepend)
	}

	return nil
}

// envValFunc is a callback used in [[envVar]] to merge existing
// values in environment values with previous ones if they were set.
//
// The input value to this callback function is the original env.Value
// and will be empty string if there is no previous value.
type envValFunc func(string) string

func identityValFunc(s string) envValFunc {
	return func(string) string { return s }
}

func trueValFunc() envValFunc {
	return identityValFunc("true")
}

func joinValFunc(value string, separator string) envValFunc {
	return func(predefinedVal string) string {
		if predefinedVal == "" {
			return value
		}
		return predefinedVal + separator + value
	}
}

func javaEnvValFunc(predefinedVal string) string {
	return predefinedVal + javaToolOptionsValue
}

func jsEnvValFunc(predefinedVal string) string {
	return predefinedVal + nodeOptionsValue
}

func pythonEnvValFunc(predefinedVal string) string {
	if predefinedVal == "" {
		return pythonPathValue
	}
	return fmt.Sprintf("%s:%s", pythonPathValue, predefinedVal)
}

func dotnetProfilingLdPreloadEnvValFunc(predefinedVal string) string {
	if predefinedVal == "" {
		return dotnetProfilingLdPreloadValue
	}
	return fmt.Sprintf("%s:%s", dotnetProfilingLdPreloadValue, predefinedVal)
}

func rubyEnvValFunc(predefinedVal string) string {
	return predefinedVal + rubyOptValue
}

func useExistingEnvValOr(newVal string) func(string) string {
	return func(predefinedVal string) string {
		if predefinedVal != "" {
			return predefinedVal
		}
		return newVal
	}
}

func valueOrZero[T any](pointer *T) T {
	var val T
	if pointer != nil {
		val = *pointer
	}
	return val
}
