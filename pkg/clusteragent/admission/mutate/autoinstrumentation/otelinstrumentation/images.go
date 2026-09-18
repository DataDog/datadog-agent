// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package otelinstrumentation

import (
	"github.com/distribution/reference"
	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// The community SDK image defaults, which upstream keeps in two places: the repository
// is built in its config package as
// "ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-<lang>", and the
// tag comes from versions.txt at the top of its repository.
//
// This table exists because upstream's *Instrumentation defaulting webhook* is what
// fills in an empty spec.<lang>.image, and full replacement removes that webhook. Two
// callers need it:
//
//   - passthrough mode, which has to inject a community SDK image and finds none in a
//     custom resource created after the community Operator was removed;
//   - the mode discriminator, which recognises upstream's image *repository* to tell a
//     deliberate choice from a copied default when no defaulting annotation is left to
//     compare against (see mode.go).
//
// A future defaulting webhook of our own writes the same values into the custom
// resource, so the table stays the single source of truth for all three.
const (
	// upstreamImageRepositoryPrefix is completed by the language name. Note it is the
	// *operator's* image namespace, not the SDK's own: even the language-specific
	// images are published under opentelemetry-operator.
	upstreamImageRepositoryPrefix = "ghcr.io/open-telemetry/opentelemetry-operator/autoinstrumentation-"
)

// defaultImageTags is versions.txt, restricted to the languages this package maps, as of
// community Operator release 0.158.0 — which is the versions.txt to diff against when
// refreshing the table, not a tag itself, since upstream versions each SDK image
// independently of the operator.
//
// Refreshing these means following the same rule upstream follows for a customer: the
// tag moves when the operator release moves, and a customer who never pinned an image
// gets the new SDK. Left stale, we inject an older SDK than the community Operator
// would have, which is a fidelity gap rather than a breakage.
var defaultImageTags = map[Language]string{
	Java:   "2.30.0",
	NodeJS: "0.78.0",
	Python: "0.64b0",
	DotNet: "1.16.0",
}

// DefaultImage returns the community SDK image upstream's defaulting webhook would have
// written into spec.<lang>.image. The boolean is false for a language the table does not
// cover, which no MappableLanguages entry does today.
func DefaultImage(language Language) (string, bool) {
	tag, ok := defaultImageTags[language]
	if !ok {
		return "", false
	}
	return upstreamImageRepositoryPrefix + string(language) + ":" + tag, true
}

// defaultImageAnnotation returns the annotation upstream stamps on the *custom resource*
// to record the default image it used, upstream's
// AnnotationDefaultAutoInstrumentation<Lang>.
//
// It lives here rather than in annotations.go because it is not part of the pod
// targeting contract: it is written by the operator onto the custom resource, and read
// back by its own auto-upgrade sweep. All four mappable languages follow this shape,
// including nodejs, whose annotation uses the OpenTelemetry spelling and not Datadog's
// "js".
func (l Language) defaultImageAnnotation() string {
	return annotationPrefix + "default-auto-instrumentation-" + string(l) + "-image"
}

// languageImage returns the per-language image field of the custom resource, the
// counterpart of languageEnv.
func languageImage(spec otelv1alpha1.InstrumentationSpec, language Language) string {
	switch language {
	case Java:
		return spec.Java.Image
	case NodeJS:
		return spec.NodeJS.Image
	case Python:
		return spec.Python.Image
	case DotNet:
		return spec.DotNet.Image
	}
	return ""
}

// ResolveImage returns the community SDK image to inject for a language in passthrough
// mode: the custom resource's own image when it has one, and the default table's
// otherwise.
//
// The empty case is not an edge case but the normal state of a custom resource created
// after the community Operator was replaced, since nothing defaulted it. Falling back to
// the table is what keeps such a custom resource injectable — upstream builds its init
// container with the image field verbatim, so an empty value produces a pod the API
// server rejects outright.
//
// The boolean is false only when neither source has an image, which leaves the caller
// with nothing to inject and no way to guess.
func ResolveImage(cr *otelv1alpha1.Instrumentation, language Language) (string, bool) {
	if cr != nil {
		if image := languageImage(cr.Spec, language); image != "" {
			return image, true
		}
	}
	return DefaultImage(language)
}

// isUpstreamImage reports whether an image reference names the community SDK image for
// this language, ignoring the tag.
//
// The tag is ignored on purpose: this answers "is this upstream's image", not "is this
// upstream's current image", and it is used as a last-resort signal for a custom
// resource that carries no defaulting annotation at all (see languageMode). A customer
// who pinned an older upstream tag by hand is therefore read as not having made a
// deliberate SDK choice — the same reading upstream's auto-upgrade sweep applies when it
// rewrites such an image.
func isUpstreamImage(image string, language Language) bool {
	if image == "" {
		return false
	}
	ref, err := reference.Parse(image)
	if err != nil {
		log.Debugf("Cannot parse Instrumentation %s image reference %q: %v", language, image, err)
		return false
	}
	named, ok := ref.(reference.Named)
	if !ok {
		return false
	}
	return named.Name() == upstreamImageRepositoryPrefix+string(language)
}
