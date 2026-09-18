// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"slices"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/otelinstrumentation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// otelTargetName identifies an injection target built from the community
// OpenTelemetry Operator's Instrumentation custom resource, as opposed to a
// configuration target or a Remote Config policy.
const otelTargetName = "otel-instrumentation-crd"

// otelAnnotationResult reports what a pod's community OpenTelemetry annotations ask
// for, shaped as an annotationResult so the precedence chain can return it directly.
//
// A nil return means the OpenTelemetry contract has nothing to say about this pod and
// the caller must carry on with the existing precedence chain, unchanged. A non-nil
// return always decides — it never lets the chain continue — and carries either a
// target built from the Instrumentation custom resource, or no target at all, which
// leaves the pod uninstrumented deliberately.
func (m *TargetMutator) otelAnnotationResult(pod *corev1.Pod) *annotationResult {
	if m.otelResolver == nil {
		return nil
	}

	result := m.otelResolver.Resolve(pod, pod.Namespace)
	switch result.Outcome {
	case otelinstrumentation.OutcomeNoAnnotation:
		return nil
	case otelinstrumentation.OutcomeUnresolvable:
		// Injecting nothing is the faithful answer, and it has to stop the chain:
		// falling through to targets or Remote Config would instrument a pod the
		// community Operator would have left alone.
		//
		// Warn, not Debug: the pod asked for instrumentation and got none, which is
		// invisible on the pod itself. Upstream logs the same situation at error level
		// for the same reason. A namespace misconfigured at scale makes this noisy,
		// which is what the telemetry counter is for.
		log.Warnf("Pod %q asks for OpenTelemetry instrumentation that cannot be resolved (%s), injecting nothing",
			mutatecommon.PodString(pod), result.Reason)
		return &annotationResult{shouldContinue: false}
	}

	// The two modes are served by different machinery and a pod can need both, one
	// language each: swap becomes a target the Datadog pipeline injects, passthrough
	// becomes upstream's own mutation, applied as is.
	target := m.otelTarget(otelinstrumentation.Translate(result, pod))
	injections := otelinstrumentation.BuildInjections(result, pod)
	if target == nil && len(injections) == 0 {
		// Resolved, yet nothing injectable came out — every requested language
		// selected containers the pod does not have, for instance. Same reasoning as
		// the unresolvable case, log level included: stop, do not fall back.
		log.Warnf("Pod %q resolved to an Instrumentation custom resource that translates to nothing injectable",
			mutatecommon.PodString(pod))
		return &annotationResult{shouldContinue: false}
	}

	return &annotationResult{shouldContinue: false, target: target, otelInjections: injections}
}

// otelTarget turns a swap-mode translation into an injection target, or nil when the
// translation asks for nothing this mutator can inject.
//
// The translation deliberately carries no library version and no image, so the choice
// of what to inject stays here, with the machinery that already knows about the
// container registry and the default library versions.
func (m *TargetMutator) otelTarget(translation otelinstrumentation.Translation) *targetInternal {
	var (
		libVersions []libInfo
		envVars     []envVar
	)

	for _, cfg := range translation.Languages {
		lang := language(cfg.Language)
		if !slices.Contains(supportedLanguages, lang) {
			// Unreachable while the two language sets agree, but injecting an unknown
			// library name would build a nonexistent image reference.
			log.Warnf("No Datadog tracing library named %q, skipping it for OpenTelemetry instrumentation", cfg.Language)
			continue
		}

		// One libInfo per container rather than one with an empty ctrName: container
		// names are matched strictly at injection time, and an empty one means "every
		// container", which would instrument containers the pod did not select.
		for _, ctrName := range cfg.Containers {
			libVersions = append(libVersions, lang.defaultLibInfo(m.containerRegistry, ctrName))
		}

		containers := make(map[string]struct{}, len(cfg.Containers))
		for _, ctrName := range cfg.Containers {
			containers[ctrName] = struct{}{}
		}
		for _, env := range cfg.EnvVars {
			envVars = append(envVars, otelEnvVarMutator(env, containers))
		}
	}

	if len(libVersions) == 0 {
		return nil
	}

	return &targetInternal{
		name:        otelTargetName,
		libVersions: libVersions,
		// usesDefaultLibs stays false: the languages come from the pod's annotations,
		// so process language detection must not second-guess them.
		containerScopedEnvVars: envVars,
	}
}

// otelEnvVarMutator is envVarMutator restricted to a set of container names.
//
// The OpenTelemetry annotation contract configures the containers it selected and no
// others, so its variables cannot go through targetInternal.envVars, which is applied
// to the whole pod. Leaking DD_SERVICE or DD_TAGS into containers the user did not
// select would change how the rest of Datadog sees them.
func otelEnvVarMutator(env corev1.EnvVar, containers map[string]struct{}) envVar {
	mutator := envVarMutator(env)
	mutator.isEligibleToInject = func(c *corev1.Container) bool {
		_, selected := containers[c.Name]
		return selected
	}
	return mutator
}
