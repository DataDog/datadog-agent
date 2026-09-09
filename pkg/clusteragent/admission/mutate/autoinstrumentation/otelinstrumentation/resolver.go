// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package otelinstrumentation

import (
	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	corev1 "k8s.io/api/core/v1"
)

// Outcome is what the OpenTelemetry annotation contract says about a pod. The three
// values are not interchangeable and callers must branch on all of them.
type Outcome string

const (
	// OutcomeNoAnnotation means the pod requests no OpenTelemetry instrumentation for
	// any mappable language. The caller must carry on with the existing SSI precedence
	// chain exactly as if this feature did not exist.
	OutcomeNoAnnotation Outcome = "no_annotation"

	// OutcomeResolved means every requested language resolved to an Instrumentation
	// custom resource.
	OutcomeResolved Outcome = "resolved"

	// OutcomeUnresolvable means the pod asks for OpenTelemetry instrumentation but the
	// request cannot be honoured. The caller must inject nothing at all and must *not*
	// fall back to Datadog targets or Remote Config: upstream leaves such a pod
	// uninstrumented, and falling back would instrument a pod the community Operator
	// would have left alone. This is the reason OutcomeNoAnnotation and
	// OutcomeUnresolvable have to stay distinct.
	OutcomeUnresolvable Outcome = "unresolvable"
)

// Reason explains an OutcomeUnresolvable. The values are low cardinality and safe as a
// telemetry tag.
type Reason string

const (
	// ReasonStoreUnavailable means the store is inert or has not synced, so no custom
	// resource could be read. Distinct from ReasonNoInstrumentationInNamespace because
	// it is a Cluster Agent condition, not a cluster configuration mistake.
	ReasonStoreUnavailable Reason = "store_unavailable"
	// ReasonNoInstrumentationInNamespace means "true" was requested but the pod's
	// namespace holds no Instrumentation custom resource.
	ReasonNoInstrumentationInNamespace Reason = "no_instrumentation_in_namespace"
	// ReasonMultipleInstrumentations means "true" was requested but the pod's namespace
	// holds more than one Instrumentation custom resource, so the choice is ambiguous.
	ReasonMultipleInstrumentations Reason = "multiple_instrumentations_in_namespace"
	// ReasonInstrumentationNotFound means a named Instrumentation custom resource does
	// not exist.
	ReasonInstrumentationNotFound Reason = "instrumentation_not_found"
	// ReasonInvalidContainerNames means a container selection annotation failed
	// upstream's ^[a-zA-Z0-9-,]+$ validation.
	ReasonInvalidContainerNames Reason = "invalid_container_names"
)

// LanguageRequest is one language's resolved configuration.
//
// Each language carries its own custom resource because upstream resolves the
// inject-<lang> annotations independently: a pod may legitimately point java and python
// at different Instrumentation custom resources. In practice they almost always name
// the same one, but collapsing them here would be a lie the translation pass could not
// undo.
type LanguageRequest struct {
	Language Language
	// Instrumentation is the custom resource this language resolved to. Never nil.
	Instrumentation *otelv1alpha1.Instrumentation
	// Mode is what to inject for this language: the configured mode, unless the custom
	// resource's image overrides it. Resolved per language because spec.<lang>.image is
	// per language, so one custom resource can legitimately ask for a community SDK for
	// java and leave python on the configured default.
	Mode Mode
	// ModeSource records why Mode came out that way, for telemetry and debugging.
	ModeSource ModeSource
	// Containers are the container names selected for this language, the common
	// container-names annotation followed by the per-language one. Empty means the pod
	// made no selection; upstream then defaults to the first regular container, which
	// is a decision for the translation pass, not for resolution.
	Containers []string
}

// Result is the outcome of resolving a pod against the OpenTelemetry annotation
// contract. It is a tagged union: read Languages only when Outcome is OutcomeResolved,
// and Reason only when it is OutcomeUnresolvable.
type Result struct {
	Outcome Outcome

	// Languages holds one entry per requested language, in MappableLanguages order.
	Languages []LanguageRequest

	// Reason and ReasonLanguage describe an OutcomeUnresolvable. ReasonLanguage is the
	// language that failed first, and is empty when the failure is not specific to one
	// (an invalid common container-names annotation, for instance).
	Reason         Reason
	ReasonLanguage Language
}

// NamespaceAnnotationGetter reads a namespace's annotations from a local cache.
//
// Namespace-level annotations are half of the upstream contract, but the admission path
// only receives a namespace *name* and must not issue an API call, so the lookup is left
// to the caller. A nil getter is valid and degrades resolution to pod annotations only:
// every namespace then looks unannotated, which by effectiveAnnotationValue's rules
// means the pod value always wins.
type NamespaceAnnotationGetter interface {
	// NamespaceAnnotations returns the annotations of the named namespace. The boolean
	// reports whether they are known; false must be returned rather than an error when
	// the namespace is absent from the cache, so that resolution never blocks admission.
	NamespaceAnnotations(namespace string) (map[string]string, bool)
}

// instrumentationLookup is the part of Store the resolver depends on.
type instrumentationLookup interface {
	Get(namespace, name string) (*otelv1alpha1.Instrumentation, bool)
	ListNamespace(namespace string) ([]*otelv1alpha1.Instrumentation, bool)
}

// Resolver answers what the OpenTelemetry annotation contract asks for a given pod,
// using only in-memory caches.
type Resolver struct {
	store      instrumentationLookup
	namespaces NamespaceAnnotationGetter
	mode       Mode
	telemetry  *resolverTelemetry
}

// NewResolver returns a Resolver reading custom resources from store. namespaces may be
// nil, in which case namespace-level annotations are ignored.
//
// mode is the configured default, which a custom resource naming its own SDK image
// overrides per language. Callers construct a Resolver only when the mode is enabled;
// passing ModeDisabled is not rejected but resolves every language to ModeDisabled,
// leaving annotated pods uninstrumented rather than half-instrumented.
func NewResolver(store *Store, namespaces NamespaceAnnotationGetter, mode Mode) *Resolver {
	return &Resolver{
		store:      store,
		namespaces: namespaces,
		mode:       mode,
		telemetry:  newResolverTelemetry(),
	}
}

// Resolve reports what the OpenTelemetry annotation contract asks for this pod. It only
// reads in-memory caches and never returns an error: a request that cannot be honoured
// is an OutcomeUnresolvable result, because failing admission over a third-party
// annotation is never the right answer.
//
// The order of operations mirrors upstream's Mutate: resolve every language's custom
// resource first, bail out with OutcomeNoAnnotation if none was requested, and only then
// look at the container selection annotations. That last ordering matters — a pod with a
// malformed container-names annotation but no inject-<lang> annotation falls through
// untouched, because upstream never validates it.
func (r *Resolver) Resolve(pod *corev1.Pod, namespace string) Result {
	podAnnotations := pod.Annotations
	namespaceAnnotations := r.namespaceAnnotations(namespace)

	requests := make([]LanguageRequest, 0, len(MappableLanguages))
	for _, language := range MappableLanguages {
		value := effectiveAnnotationValue(podAnnotations, namespaceAnnotations, language.injectAnnotation())
		request := parseInjectValue(value, namespace)

		cr, reason := r.lookup(request, namespace)
		if reason != "" {
			return r.unresolvable(reason, language)
		}
		if cr == nil {
			continue
		}

		mode, source := languageMode(cr, language, r.mode)
		r.telemetry.recordMode(mode, source, language)
		requests = append(requests, LanguageRequest{
			Language:        language,
			Instrumentation: cr,
			Mode:            mode,
			ModeSource:      source,
		})
	}

	if len(requests) == 0 {
		return Result{Outcome: OutcomeNoAnnotation}
	}

	common, err := parseContainerNames(
		effectiveAnnotationValue(podAnnotations, namespaceAnnotations, commonContainerNamesAnnotation))
	if err != nil {
		return r.unresolvable(ReasonInvalidContainerNames, "")
	}

	for i := range requests {
		language := requests[i].Language
		perLanguage, err := parseContainerNames(
			effectiveAnnotationValue(podAnnotations, namespaceAnnotations, language.containerNamesAnnotation()))
		if err != nil {
			return r.unresolvable(ReasonInvalidContainerNames, language)
		}

		// Upstream assigns the common names and then appends the per-language ones, so
		// both apply rather than the more specific one replacing the other.
		containers := make([]string, 0, len(common)+len(perLanguage))
		containers = append(containers, common...)
		containers = append(containers, perLanguage...)
		if len(containers) > 0 {
			requests[i].Containers = containers
		}
	}

	return Result{Outcome: OutcomeResolved, Languages: requests}
}

// lookup resolves one language's request to a custom resource. A nil custom resource
// with an empty reason means the language was not requested.
func (r *Resolver) lookup(request injectRequest, namespace string) (*otelv1alpha1.Instrumentation, Reason) {
	switch request.directive {
	case directiveNone, directiveDisabled:
		return nil, ""

	case directiveSoleInNamespace:
		crs, serving := r.store.ListNamespace(namespace)
		if !serving {
			return nil, ReasonStoreUnavailable
		}
		switch len(crs) {
		case 0:
			return nil, ReasonNoInstrumentationInNamespace
		case 1:
			return crs[0], ""
		default:
			// Upstream has no default custom resource name to fall back on, so several
			// candidates is genuinely ambiguous rather than a tie to break.
			return nil, ReasonMultipleInstrumentations
		}

	case directiveNamed:
		cr, found := r.store.Get(request.namespace, request.name)
		if !found {
			// A miss here can also mean the store is inert, but the two are not worth
			// distinguishing: either way the named custom resource is unavailable.
			return nil, ReasonInstrumentationNotFound
		}
		return cr, ""
	}

	return nil, ""
}

func (r *Resolver) namespaceAnnotations(namespace string) map[string]string {
	if r.namespaces == nil {
		return nil
	}
	annotations, ok := r.namespaces.NamespaceAnnotations(namespace)
	if !ok {
		return nil
	}
	return annotations
}

func (r *Resolver) unresolvable(reason Reason, language Language) Result {
	r.telemetry.recordUnresolvable(reason, language)
	return Result{Outcome: OutcomeUnresolvable, Reason: reason, ReasonLanguage: language}
}
