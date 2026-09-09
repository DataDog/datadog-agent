// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package otelinstrumentation

import (
	"testing"

	otelv1alpha1 "github.com/open-telemetry/opentelemetry-operator/apis/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseMode(t *testing.T) {
	tests := []struct {
		value   string
		want    Mode
		wantErr bool
	}{
		{value: "disabled", want: ModeDisabled},
		{value: "otel", want: ModeOTel},
		{value: "datadog", want: ModeDatadog},
		// An unset setting and an explicitly disabled one must agree.
		{value: "", want: ModeDisabled},
		// A typo must be rejected rather than defaulted, or it silently turns the
		// feature off or picks the wrong SDK.
		{value: "Datadog", want: ModeDisabled, wantErr: true},
		{value: "enabled", want: ModeDisabled, wantErr: true},
		{value: "true", want: ModeDisabled, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			mode, err := ParseMode(tt.value)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tt.want, mode)
		})
	}
}

func TestModeEnabled(t *testing.T) {
	assert.False(t, ModeDisabled.Enabled())
	assert.True(t, ModeOTel.Enabled())
	assert.True(t, ModeDatadog.Enabled())
}

// crWith builds a custom resource with one language's image and, optionally, the
// defaulting annotation upstream would have stamped.
func crWith(language Language, image, stampedImage string) *otelv1alpha1.Instrumentation {
	cr := &otelv1alpha1.Instrumentation{
		ObjectMeta: metav1.ObjectMeta{Namespace: "workloads", Name: "default"},
	}
	if stampedImage != "" {
		cr.Annotations = map[string]string{language.defaultImageAnnotation(): stampedImage}
	}

	switch language {
	case Java:
		cr.Spec.Java.Image = image
	case NodeJS:
		cr.Spec.NodeJS.Image = image
	case Python:
		cr.Spec.Python.Image = image
	case DotNet:
		cr.Spec.DotNet.Image = image
	}
	return cr
}

func TestLanguageMode(t *testing.T) {
	upstreamCurrent, ok := DefaultImage(Python)
	require.True(t, ok)
	upstreamOlder := upstreamImageRepositoryPrefix + "python:0.50b0"
	const userImage = "registry.example.com/our-python-sdk:1.2.3"

	tests := []struct {
		name       string
		image      string
		stamped    string
		configured Mode
		wantMode   Mode
		wantSource ModeSource
	}{
		{
			name:       "no image expresses no preference",
			configured: ModeDatadog,
			wantMode:   ModeDatadog,
			wantSource: ModeSourceNoImage,
		},
		{
			name:       "no image in passthrough stays in passthrough",
			configured: ModeOTel,
			wantMode:   ModeOTel,
			wantSource: ModeSourceNoImage,
		},
		{
			// The common case for a custom resource the community Operator admitted:
			// the annotation proves the image is its default, not a user choice.
			name:       "an image matching the defaulting annotation is not a choice",
			image:      upstreamCurrent,
			stamped:    upstreamCurrent,
			configured: ModeDatadog,
			wantMode:   ModeDatadog,
			wantSource: ModeSourceOperatorDefault,
		},
		{
			// Still true when the operator's default has moved on since: what matters
			// is that the image equals what *this* custom resource was defaulted with.
			name:       "an older operator default is still a default",
			image:      upstreamOlder,
			stamped:    upstreamOlder,
			configured: ModeDatadog,
			wantMode:   ModeDatadog,
			wantSource: ModeSourceOperatorDefault,
		},
		{
			name:       "an image differing from the annotation is deliberate",
			image:      userImage,
			stamped:    upstreamCurrent,
			configured: ModeDatadog,
			wantMode:   ModeOTel,
			wantSource: ModeSourceUserImage,
		},
		{
			// A private mirror of upstream's image: the name test would miss it, the
			// annotation does not.
			name:       "a mirrored upstream image differing from the annotation is deliberate",
			image:      "registry.example.com/mirrors/autoinstrumentation-python:" + defaultImageTags[Python],
			stamped:    upstreamCurrent,
			configured: ModeDatadog,
			wantMode:   ModeOTel,
			wantSource: ModeSourceUserImage,
		},
		{
			// No webhook ever ran on this custom resource, so the name is all there is.
			name:       "upstream's own image without an annotation is read as a default",
			image:      upstreamCurrent,
			configured: ModeDatadog,
			wantMode:   ModeDatadog,
			wantSource: ModeSourceUpstreamImageName,
		},
		{
			name:       "an unrecognised image without an annotation is read as deliberate",
			image:      userImage,
			configured: ModeDatadog,
			wantMode:   ModeOTel,
			wantSource: ModeSourceUnknownImage,
		},
		{
			// An unrecognised image already asks for passthrough, so a passthrough
			// default changes nothing — the source still says the image decided.
			name:       "an unrecognised image in passthrough stays in passthrough",
			image:      userImage,
			configured: ModeOTel,
			wantMode:   ModeOTel,
			wantSource: ModeSourceUnknownImage,
		},
		{
			// An empty annotation is treated as absent, as upstream's auto-upgrade
			// sweep does, so the decision falls through to the name test.
			name:       "an empty annotation is treated as absent",
			image:      upstreamCurrent,
			stamped:    "",
			configured: ModeDatadog,
			wantMode:   ModeDatadog,
			wantSource: ModeSourceUpstreamImageName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, source := languageMode(crWith(Python, tt.image, tt.stamped), Python, tt.configured)
			assert.Equal(t, tt.wantMode, mode)
			assert.Equal(t, tt.wantSource, source)
		})
	}
}

// A disabled mode reaching resolution must not be read as an enabled one: leaving the
// pod alone is the safe degradation, injecting a library nobody asked for is not.
func TestLanguageModeDisabledConfiguredMode(t *testing.T) {
	mode, source := languageMode(crWith(Python, "", ""), Python, ModeDisabled)
	assert.Equal(t, ModeDisabled, mode)
	assert.Equal(t, ModeSourceNoImage, source)
}

func TestLanguageModeNilCustomResource(t *testing.T) {
	mode, source := languageMode(nil, Python, ModeDatadog)
	assert.Equal(t, ModeDatadog, mode)
	assert.Equal(t, ModeSourceNoImage, source)
}

// The decision is per language: one language pinning its own SDK image must not drag the
// others into passthrough.
func TestLanguageModeIsPerLanguage(t *testing.T) {
	cr := crWith(Python, "registry.example.com/our-python-sdk:1", "")

	mode, source := languageMode(cr, Python, ModeDatadog)
	assert.Equal(t, ModeOTel, mode)
	assert.Equal(t, ModeSourceUnknownImage, source)

	for _, language := range []Language{Java, NodeJS, DotNet} {
		mode, source := languageMode(cr, language, ModeDatadog)
		assert.Equalf(t, ModeDatadog, mode, "language %s", language)
		assert.Equalf(t, ModeSourceNoImage, source, "language %s", language)
	}
}
