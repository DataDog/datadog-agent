// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package otelinstrumentation

import (
	"fmt"
	"regexp"
	"strings"
)

// annotationPrefix is the community OpenTelemetry Operator's annotation namespace.
const annotationPrefix = "instrumentation.opentelemetry.io/"

// commonContainerNamesAnnotation selects containers for every enabled language.
const commonContainerNamesAnnotation = annotationPrefix + "container-names"

// Language is one of the OpenTelemetry auto-instrumentation languages that has a
// Datadog tracing library equivalent.
type Language string

const (
	// Java is the java auto-instrumentation language.
	Java Language = "java"
	// NodeJS is the nodejs auto-instrumentation language, which Datadog calls "js".
	NodeJS Language = "nodejs"
	// Python is the python auto-instrumentation language.
	Python Language = "python"
	// DotNet is the dotnet auto-instrumentation language.
	DotNet Language = "dotnet"
)

// MappableLanguages are the languages both the OpenTelemetry CRD and Datadog SSI
// support, in the order the community Operator evaluates them. The order is
// behaviour, not cosmetics: upstream aborts the whole pod on the first language whose
// custom resource cannot be resolved, so it decides which reason gets reported.
//
// The languages left out are not a choice: go, apacheHttpd and nginx have no SSI
// equivalent, and ruby, php and c have no block in the OpenTelemetry CRD to read.
var MappableLanguages = []Language{Java, NodeJS, Python, DotNet}

// injectAnnotation returns the inject-<lang> annotation key for the language.
func (l Language) injectAnnotation() string {
	return annotationPrefix + "inject-" + string(l)
}

// containerNamesAnnotation returns the per-language container selection annotation key.
//
// All four mappable languages follow the <lang>-container-names shape. Upstream is not
// consistent beyond them: nginx uses inject-nginx-container-names. Extending
// MappableLanguages means special-casing that key.
func (l Language) containerNamesAnnotation() string {
	return annotationPrefix + string(l) + "-container-names"
}

// injectDirective is what an inject-<lang> annotation value asks for.
type injectDirective int

const (
	// directiveNone means the annotation is absent or empty: this language is not requested.
	directiveNone injectDirective = iota
	// directiveDisabled means the value is "false".
	directiveDisabled
	// directiveSoleInNamespace means the value is "true": use the one custom resource in
	// the pod's namespace, of which there must be exactly one.
	directiveSoleInNamespace
	// directiveNamed means the value names a custom resource, optionally namespace-qualified.
	directiveNamed
)

// injectRequest is a parsed inject-<lang> annotation value.
type injectRequest struct {
	directive injectDirective
	// namespace and name are only meaningful for directiveNamed.
	namespace string
	name      string
}

// parseInjectValue parses an inject-<lang> annotation value, mirroring upstream's
// getInstrumentationInstance. podNamespace is used when the value names a custom
// resource without qualifying its namespace.
//
// Anything that is not empty, "true" or "false" is a custom resource name, so values
// like "yes" or "1" are names that will simply not be found — upstream behaves the same
// way.
func parseInjectValue(value, podNamespace string) injectRequest {
	switch {
	case value == "":
		return injectRequest{directive: directiveNone}
	case strings.EqualFold(value, "false"):
		return injectRequest{directive: directiveDisabled}
	case strings.EqualFold(value, "true"):
		return injectRequest{directive: directiveSoleInNamespace}
	}

	// Split on the first "/" only, as strings.Cut does upstream.
	if namespace, name, qualified := strings.Cut(value, "/"); qualified {
		return injectRequest{directive: directiveNamed, namespace: namespace, name: name}
	}
	return injectRequest{directive: directiveNamed, namespace: podNamespace, name: value}
}

// effectiveAnnotationValue combines a pod and a namespace annotation into the value
// upstream would act on. It is a faithful port of the community Operator's
// annotationValue, and it applies to the container selection annotations as much as to
// the inject-<lang> ones.
//
// The rule that surprises people: "pod wins" is not the whole story. A pod value of
// "true" only says "yes, instrument me" and leaves the choice of custom resource to the
// namespace, so a namespace naming a custom resource beats it. Any other pod value —
// "false" or an explicit name — is final.
func effectiveAnnotationValue(podAnnotations, namespaceAnnotations map[string]string, key string) string {
	podValue := podAnnotations[key]
	namespaceValue := namespaceAnnotations[key]

	if namespaceValue == "" {
		return podValue
	}
	if podValue == "" {
		return namespaceValue
	}
	if !strings.EqualFold(podValue, "true") {
		return podValue
	}
	// The pod re-enables what the namespace turned off.
	if strings.EqualFold(namespaceValue, "false") {
		return podValue
	}
	return namespaceValue
}

// containerNamesPattern is upstream's isValidContainersAnnotation check. It rejects dots
// and underscores, which are legal in many other Kubernetes name fields.
var containerNamesPattern = regexp.MustCompile(`^[a-zA-Z0-9-,]+$`)

// parseContainerNames splits a comma-separated container selection annotation. An empty
// value selects nothing and is not an error; an invalid one is, because upstream aborts
// injection for the whole pod over it.
func parseContainerNames(value string) ([]string, error) {
	if value == "" {
		return nil, nil
	}
	if !containerNamesPattern.MatchString(value) {
		return nil, fmt.Errorf("invalid characters in container names annotation %q", value)
	}
	return strings.Split(value, ","), nil
}
