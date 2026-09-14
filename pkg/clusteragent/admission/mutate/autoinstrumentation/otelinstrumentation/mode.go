// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package otelinstrumentation

import (
	"fmt"

	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
)

// Mode is what a pod resolving to an Instrumentation custom resource gets injected.
//
// The feature switch and the mode selector are one setting rather than a boolean plus a
// mode, because both non-disabled values need the same custom resource watch and only
// the translation differs. Splitting them would allow a state that means nothing —
// "passthrough, but custom resources are not read".
type Mode string

const (
	// ModeDisabled ignores the custom resources entirely: nothing is watched, and
	// Single Step Instrumentation behaves as if this feature did not exist.
	ModeDisabled Mode = "disabled"

	// ModeOTel is passthrough: inject the community SDK image and pass the custom
	// resource's OpenTelemetry configuration through unchanged.
	ModeOTel Mode = "otel"

	// ModeDatadog is swap: inject the Datadog tracing library, configured to match the
	// custom resource.
	ModeDatadog Mode = "datadog"
)

// ParseMode validates a configured mode. An empty value is ModeDisabled, so an unset
// setting and an explicitly disabled one agree.
func ParseMode(value string) (Mode, error) {
	switch mode := Mode(value); mode {
	case "":
		return ModeDisabled, nil
	case ModeDisabled, ModeOTel, ModeDatadog:
		return mode, nil
	default:
		return ModeDisabled, fmt.Errorf("invalid mode %q, expected one of %q, %q or %q",
			value, ModeDisabled, ModeOTel, ModeDatadog)
	}
}

// Enabled reports whether the mode reads custom resources at all.
func (m Mode) Enabled() bool {
	return m == ModeOTel || m == ModeDatadog
}

// ModeSource says how a language's mode was decided. The values are low cardinality and
// safe as a telemetry tag, and they are the answer to "why did this pod get that SDK",
// which is otherwise only reconstructable by hand from the custom resource.
type ModeSource string

const (
	// ModeSourceNoImage means the custom resource names no image for this language, so
	// it expressed no preference and the configured mode stands.
	ModeSourceNoImage ModeSource = "no_image"

	// ModeSourceOperatorDefault means the image is exactly what upstream's defaulting
	// webhook wrote, as proven by its own annotation, so it is not a user choice and
	// the configured mode stands.
	ModeSourceOperatorDefault ModeSource = "operator_default"

	// ModeSourceUserImage means the image differs from the one upstream's annotation
	// records, so the user chose it and passthrough honours that choice.
	ModeSourceUserImage ModeSource = "user_image"

	// ModeSourceUpstreamImageName means no defaulting annotation was found, but the
	// image names upstream's own repository, so it is read as a copied default rather
	// than a choice and the configured mode stands.
	ModeSourceUpstreamImageName ModeSource = "upstream_image_name"

	// ModeSourceUnknownImage means no defaulting annotation was found and the image is
	// not upstream's, which is the one case decided by guessing. Read as deliberate,
	// because an unrecognised image is far more likely to be a private build of a
	// community SDK than an accident.
	ModeSourceUnknownImage ModeSource = "unknown_image"
)

// languageMode decides which mode to apply to one language of a resolved custom
// resource, given the configured default.
//
// The configured mode is only a default: spec.<lang>.image is a stronger signal, because
// an image the user chose deliberately is a request for the community SDK, and swapping
// it for a Datadog library would ignore what they asked for.
//
// Telling a deliberate image from a defaulted one needs no guesswork in the common case.
// Upstream's defaulting webhook stamps
// instrumentation.opentelemetry.io/default-auto-instrumentation-<lang>-image with the
// image it used — unconditionally, even when the user supplied one — so comparing the
// two fields is exact. It is also what upstream's own auto-upgrade sweep uses to decide
// whether it may rewrite an image, which makes matching that comparison the faithful
// behaviour rather than a convenient one.
//
// Only a custom resource with no such annotation is decided by heuristic, and that is
// precisely the post-replacement world where no defaulting webhook runs. Keeping the
// name test last matters: the cases it gets wrong — upstream's image mirrored into a
// private registry, an old upstream tag pinned on purpose — carry the annotation
// whenever the community Operator ever admitted the custom resource, and so never reach
// it.
//
// One blind spot is shared with upstream by construction: a user who types exactly the
// operator's current default image is read as having been defaulted. Upstream's
// auto-upgrade sweep treats that case the same way and rewrites the image, so agreeing
// with it is the correct answer for a drop-in replacement.
func languageMode(cr *otelv1alpha1.Instrumentation, language Language, configured Mode) (Mode, ModeSource) {
	if cr == nil {
		return configured, ModeSourceNoImage
	}

	image := languageImage(cr.Spec, language)
	if image == "" {
		return configured, ModeSourceNoImage
	}

	// An annotation present but empty is treated as absent, as upstream's sweep does.
	switch stamped := cr.Annotations[language.defaultImageAnnotation()]; {
	case stamped == "":
		if isUpstreamImage(image, language) {
			return configured, ModeSourceUpstreamImageName
		}
		return ModeOTel, ModeSourceUnknownImage
	case stamped == image:
		return configured, ModeSourceOperatorDefault
	default:
		return ModeOTel, ModeSourceUserImage
	}
}
