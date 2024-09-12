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
	key                string
	valFunc            envValFunc
	isEligibleToInject func(*corev1.Container) bool
}

// mutateContainer implements containerMutator for envVar.
func (e envVar) mutateContainer(c *corev1.Container) error {
	if e.isEligibleToInject != nil && !e.isEligibleToInject(c) {
		return nil
	}

	index := slices.IndexFunc(c.Env, func(ev corev1.EnvVar) bool {
		return ev.Name == e.key
	})
	if index < 0 {
		c.Env = append(c.Env, corev1.EnvVar{
			Name:  e.key,
			Value: e.valFunc(""),
		})
	} else {
		if c.Env[index].ValueFrom != nil {
			return fmt.Errorf("%q is defined via ValueFrom", e.key)
		}
		c.Env[index].Value = e.valFunc(c.Env[index].Value)
	}

	return nil
}

type envValFunc func(string) string

func identityValFunc(s string) envValFunc {
	return func(string) string { return s }
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
