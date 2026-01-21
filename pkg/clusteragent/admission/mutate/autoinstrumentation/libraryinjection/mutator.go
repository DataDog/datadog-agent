// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InjectAPMLibraries performs the complete APM injection into a pod.
// This includes:
// 1. Injecting the APM injector (volumes, mounts, and provider-specific resources)
// 2. Injecting APM environment variables (LD_PRELOAD, etc.) into application containers
// 3. Injecting language-specific tracing libraries
//
// Returns an error if the injection fails.
func InjectAPMLibraries(pod *corev1.Pod, cfg LibraryInjectionConfig) error {
	// Select the provider based on the injection mode (annotation or default)
	factory := NewProviderFactory(InjectionMode(cfg.InjectionMode))
	provider := factory.GetProviderForPod(pod, cfg)

	// Inject the APM injector
	injectorResult := provider.InjectInjector(pod, cfg.Injector)

	// Handle injector result
	switch injectorResult.Status {
	case MutationStatusSkipped:
		annotation.Set(pod, annotation.InjectionError, injectorResult.Err.Error())
		return nil
	case MutationStatusError:
		metrics.LibInjectionErrors.Inc("injector", strconv.FormatBool(cfg.AutoDetected), cfg.InjectionType)
		log.Errorf("Cannot inject library injector into pod %s: %v", mutatecommon.PodString(pod), injectorResult.Err)
		return fmt.Errorf("injector injection failed: %w", injectorResult.Err)
	}

	// Set injector canonical version annotation if available
	if cfg.Injector.Package.CanonicalVersion != "" {
		annotation.Set(pod, annotation.InjectorCanonicalVersion, cfg.Injector.Package.CanonicalVersion)
	}

	// Inject APM environment variables to application containers
	injectAPMEnvVars(pod, cfg)

	// Inject language-specific libraries
	var lastError error
	for _, lib := range cfg.Libraries {
		// Validate language before injection
		if !IsLanguageSupported(lib.Language) {
			metrics.LibInjectionErrors.Inc(lib.Language, strconv.FormatBool(cfg.AutoDetected), cfg.InjectionType)
			lastError = fmt.Errorf("language %s is not supported", lib.Language)
			continue
		}

		// Copy the context from the injector result
		lib.Context = injectorResult.Context

		libResult := provider.InjectLibrary(pod, lib)
		injected := libResult.Status == MutationStatusInjected

		metrics.LibInjectionAttempts.Inc(lib.Language, strconv.FormatBool(injected), strconv.FormatBool(cfg.AutoDetected), cfg.InjectionType)

		if libResult.Status == MutationStatusInjected && lib.Package.CanonicalVersion != "" {
			annotation.Set(pod, annotation.LibraryCanonicalVersion.Format(lib.Language), lib.Package.CanonicalVersion)
		}

		if libResult.Status == MutationStatusError {
			metrics.LibInjectionErrors.Inc(lib.Language, strconv.FormatBool(cfg.AutoDetected), cfg.InjectionType)
			lastError = fmt.Errorf("library injection failed for %s: %w", lib.Language, libResult.Err)
		}
	}

	return lastError
}

// injectAPMEnvVars injects APM environment variables (LD_PRELOAD, etc.) into application containers.
func injectAPMEnvVars(pod *corev1.Pod, cfg LibraryInjectionConfig) {
	patcher := NewPodPatcher(pod, cfg.ContainerFilter)

	patcher.AddEnvVarWithJoin("LD_PRELOAD", asAbsPath(injectorFilePath("launcher.preload.so")), ":")
	patcher.AddEnvVar(corev1.EnvVar{Name: "DD_INJECT_SENDER_TYPE", Value: "k8s"})
	patcher.AddEnvVar(corev1.EnvVar{Name: "DD_INJECT_START_TIME", Value: strconv.FormatInt(time.Now().Unix(), 10)})

	if cfg.Debug {
		patcher.AddEnvVar(corev1.EnvVar{Name: "DD_APM_INSTRUMENTATION_DEBUG", Value: "true"})
		patcher.AddEnvVar(corev1.EnvVar{Name: "DD_TRACE_STARTUP_LOGS", Value: "true"})
		patcher.AddEnvVar(corev1.EnvVar{Name: "DD_TRACE_DEBUG", Value: "true"})
	}
}
