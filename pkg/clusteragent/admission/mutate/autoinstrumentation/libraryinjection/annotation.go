// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package libraryinjection

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// FIXME: This file duplicates code from pkg/clusteragent/admission/mutate/autoinstrumentation/annotation.go.
// We cannot import it directly because it would create a circular dependency:
//   - autoinstrumentation imports library_injection
//   - library_injection would import autoinstrumentation (for SetAnnotation/AnnotationInjectionError)
//
// Possible solutions:
//   1. Move shared annotation utilities to a separate package (e.g., pkg/clusteragent/admission/mutate/common)
//   2. Pass annotation setter as a callback (adds complexity)
//   3. Keep this duplication (current approach, simple but not DRY)

// Annotation keys for APM injection.
const (
	// annotationInjectionError is the annotation key used to record injection errors.
	annotationInjectionError = "internal.apm.datadoghq.com/injection-error"
	// annotationInjectorCanonicalVersion is set with the actual version of the injector.
	annotationInjectorCanonicalVersion = "internal.apm.datadoghq.com/injector-canonical-version"
	// annotationLibraryCanonicalVersionPrefix is the prefix for library canonical version annotations.
	// The full key is: internal.apm.datadoghq.com/<lang>-canonical-version
	annotationLibraryCanonicalVersionPrefix = "internal.apm.datadoghq.com/"
	// annotationLibraryCanonicalVersionSuffix is the suffix for library canonical version annotations.
	annotationLibraryCanonicalVersionSuffix = "-canonical-version"
)

// setAnnotation sets a key-value annotation on the pod.
func setAnnotation(pod *corev1.Pod, key, value string) {
	if pod.Annotations == nil {
		pod.Annotations = make(map[string]string)
	}
	pod.Annotations[key] = value
	log.Debugf("Set annotation %s=%s for Single Step Instrumentation.", key, value)
}
