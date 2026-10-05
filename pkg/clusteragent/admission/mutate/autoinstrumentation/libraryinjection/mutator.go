// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/metrics"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/patch"
	"github.com/DataDog/datadog-agent/pkg/ssi"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// injectedLibraryEntry represents a single component injected into a pod,
// stored in the InjectedLibraries annotation.
type injectedLibraryEntry struct {
	// Name is the component name: "injector" or a language (e.g. "java", "python").
	Name string `json:"name"`
	// Image is the full OCI image reference used for injection.
	Image string `json:"image"`
	// Status is the outcome of the injection attempt: "injected", "skipped", or "error".
	Status string `json:"status"`
}

// PlanAPMLibraries performs the complete APM injection into a pod.
// This includes:
// 1. Injecting the APM injector (volumes, mounts, and provider-specific resources)
// 2. Injecting APM environment variables (LD_PRELOAD, etc.) into application containers
// 3. Injecting language-specific tracing libraries
//
// Returns an error if the injection fails.
func PlanAPMLibraries(session *patch.PodSession, cfg LibraryInjectionConfig) error {
	pod, err := session.Snapshot()
	if err != nil {
		return err
	}
	injectionStatus := annotation.InjectionStatusError
	var injectionErr error

	run := func() error {

		// Record the observed state of the Datadog CSI driver when detection is active.
		// This is set regardless of the configured injection mode so that an operator
		// can answer "is the driver installed?" and "is APM support enabled?" from the
		// pod annotations alone.
		if w := cfg.CSIDriverWatcher; w != nil {
			var csiStatus string
			switch {
			case w.IsAPMEnabled():
				csiStatus = annotation.CSIDriverStatusAPMEnabled
			case w.IsRegistered():
				csiStatus = annotation.CSIDriverStatusAPMDisabled
			default:
				csiStatus = annotation.CSIDriverStatusNotInstalled
			}

			if err := annotation.SetPatch(session, annotation.CSIDriverStatus, csiStatus); err != nil {
				return err
			}
		}

		// Select the provider based on the injection mode (annotation or default)
		factory := NewProviderFactory(InjectionMode(cfg.InjectionMode))
		provider := factory.GetProviderForPod(pod, cfg)

		if err := annotation.SetPatch(session, annotation.EffectiveInjectionMode, provider.GetName()); err != nil {
			return err
		}

		// Inject the APM injector
		injectorResult := provider.PlanInjector(session, cfg.Injector)
		if err := session.Err(); err != nil {
			return err
		}

		// Handle injector result
		switch injectorResult.Status {
		case MutationStatusSkipped:
			injectionErr = injectorResult.Err
			injectionStatus = annotation.InjectionStatusSkipped
			return nil
		case MutationStatusError:
			metrics.LibInjectionErrors.Inc("injector", strconv.FormatBool(cfg.AutoDetected), cfg.InjectionType)
			log.Errorf("Cannot inject library injector into pod %s: %v", mutatecommon.PodString(pod), injectorResult.Err)
			injectionErr = injectorResult.Err
			return fmt.Errorf("injector injection failed: %w", injectorResult.Err)
		}

		// Set injector canonical version annotation if available
		if cfg.Injector.Package.CanonicalVersion != "" {

			if err := annotation.SetPatch(session, annotation.InjectorCanonicalVersion, cfg.Injector.Package.CanonicalVersion); err != nil {
				return err
			}
		}

		// Inject APM environment variables to application containers
		if err := planAPMEnvVars(session, cfg); err != nil {
			return err
		}

		// Inject language-specific libraries and collect entries for the annotation.
		// All attempted libraries are recorded, regardless of outcome.
		// injectionErr is non-nil as soon as any library ends up with a non-injected status.
		injectedEntries := []injectedLibraryEntry{{Name: "injector", Image: cfg.Injector.Package.FullRef(), Status: string(MutationStatusInjected)}}
		for _, lib := range cfg.Libraries {
			injectedEntries = append(injectedEntries, injectedLibraryEntry{Name: lib.Language, Image: lib.Package.FullRef()})
			entry := &injectedEntries[len(injectedEntries)-1]

			// Validate language before injection
			if !ssi.IsLanguageSupported(lib.Language) {
				metrics.LibInjectionErrors.Inc(lib.Language, strconv.FormatBool(cfg.AutoDetected), cfg.InjectionType)
				injectionErr = fmt.Errorf("language %s is not supported", lib.Language)
				entry.Status = string(MutationStatusSkipped)
				continue
			}

			// Copy the context from the injector result
			lib.Context = injectorResult.Context

			libResult := provider.PlanLibrary(session, lib)
			if err := session.Err(); err != nil {
				return err
			}
			injected := libResult.Status == MutationStatusInjected
			entry.Status = string(libResult.Status)

			metrics.LibInjectionAttempts.Inc(lib.Language, strconv.FormatBool(injected), strconv.FormatBool(cfg.AutoDetected), cfg.InjectionType)

			if libResult.Status == MutationStatusInjected && lib.Package.CanonicalVersion != "" {

				if err := annotation.SetPatch(session, annotation.LibraryCanonicalVersion.Format(lib.Language), lib.Package.CanonicalVersion); err != nil {
					return err
				}
			}

			if libResult.Status == MutationStatusError {
				metrics.LibInjectionErrors.Inc(lib.Language, strconv.FormatBool(cfg.AutoDetected), cfg.InjectionType)
				injectionErr = fmt.Errorf("library injection failed for %s: %w", lib.Language, libResult.Err)
			}
		}

		if entriesJSON, err := json.Marshal(injectedEntries); err == nil {

			if err := annotation.SetPatch(session, annotation.InjectedLibraries, string(entriesJSON)); err != nil {
				return err
			}
		} else {
			log.Errorf("Failed to marshal injected libraries annotation for pod %s: %v", mutatecommon.PodString(pod), err)
		}

		if injectionErr != nil {
			injectionStatus = annotation.InjectionStatusPartial
		} else {
			injectionStatus = annotation.InjectionStatusInjected
		}

		return injectionErr
	}
	result := run()
	if err := session.Err(); err != nil {
		return err
	}
	values := map[string]string{annotation.InjectionStatus: injectionStatus}
	if injectionErr != nil {
		values[annotation.InjectionError] = injectionErr.Error()
	}
	if err := session.SetAnnotations(values, false); err != nil {
		return err
	}
	return result
}

// injectAPMEnvVars injects APM environment variables (LD_PRELOAD, etc.) into application containers.
func planAPMEnvVars(session *patch.PodSession, cfg LibraryInjectionConfig) error {
	patcher := NewPodPatcher(session, cfg.ContainerFilter)

	if err := patcher.AddEnvVarWithJoin("LD_PRELOAD", asAbsPath(injectorFilePath("launcher.preload.so")), ":"); err != nil {
		return err
	}

	if err := patcher.AddEnvVar(corev1.EnvVar{Name: "DD_INJECT_SENDER_TYPE", Value: "k8s"}); err != nil {
		return err
	}

	if err := patcher.AddEnvVar(corev1.EnvVar{Name: "DD_INJECT_START_TIME", Value: strconv.FormatInt(time.Now().Unix(), 10)}); err != nil {
		return err
	}

	if cfg.Debug {

		if err := patcher.AddEnvVar(corev1.EnvVar{Name: "DD_APM_INSTRUMENTATION_DEBUG", Value: "true"}); err != nil {
			return err
		}

		if err := patcher.AddEnvVar(corev1.EnvVar{Name: "DD_TRACE_STARTUP_LOGS", Value: "true"}); err != nil {
			return err
		}

		if err := patcher.AddEnvVar(corev1.EnvVar{Name: "DD_TRACE_DEBUG", Value: "true"}); err != nil {
			return err
		}
	}
	return nil
}
