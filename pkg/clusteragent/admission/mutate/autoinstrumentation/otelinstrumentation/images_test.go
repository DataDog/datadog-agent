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
)

// Every language the package claims to map must have a default image, or passthrough
// would resolve it to nothing and the mode discriminator would lose its reference point.
// This is the test that fails when a language is added to MappableLanguages alone.
func TestDefaultImageCoversEveryMappableLanguage(t *testing.T) {
	for _, language := range MappableLanguages {
		image, ok := DefaultImage(language)
		require.Truef(t, ok, "no default image for %s", language)
		assert.Equal(t, upstreamImageRepositoryPrefix+string(language)+":"+defaultImageTags[language], image)
		assert.Truef(t, isUpstreamImage(image, language), "%s default image is not recognized as upstream's", language)
	}
}

func TestDefaultImageUnknownLanguage(t *testing.T) {
	image, ok := DefaultImage(Language("ruby"))
	assert.False(t, ok)
	assert.Empty(t, image)
}

func TestResolveImage(t *testing.T) {
	javaDefault, ok := DefaultImage(Java)
	require.True(t, ok)

	tests := []struct {
		name  string
		cr    *otelv1alpha1.Instrumentation
		want  string
		wantK bool
	}{
		{
			name: "the custom resource's own image wins",
			cr: &otelv1alpha1.Instrumentation{Spec: otelv1alpha1.InstrumentationSpec{
				Java: otelv1alpha1.Java{Image: "registry.example.com/java-sdk:1.2.3"},
			}},
			want:  "registry.example.com/java-sdk:1.2.3",
			wantK: true,
		},
		{
			// The normal state of a custom resource created after the community
			// Operator was replaced: nothing defaulted spec.java.image.
			name:  "an empty image falls back to the default table",
			cr:    &otelv1alpha1.Instrumentation{},
			want:  javaDefault,
			wantK: true,
		},
		{
			name:  "a nil custom resource still resolves to the default",
			cr:    nil,
			want:  javaDefault,
			wantK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			image, ok := ResolveImage(tt.cr, Java)
			assert.Equal(t, tt.wantK, ok)
			assert.Equal(t, tt.want, image)
		})
	}
}

func TestResolveImagePerLanguage(t *testing.T) {
	// One language pinned, the rest defaulted: resolution must not leak the pinned
	// image onto the others.
	cr := &otelv1alpha1.Instrumentation{Spec: otelv1alpha1.InstrumentationSpec{
		Python: otelv1alpha1.Python{Image: "registry.example.com/python-sdk:9"},
	}}

	python, ok := ResolveImage(cr, Python)
	require.True(t, ok)
	assert.Equal(t, "registry.example.com/python-sdk:9", python)

	for _, language := range []Language{Java, NodeJS, DotNet} {
		image, ok := ResolveImage(cr, language)
		require.True(t, ok)
		want, _ := DefaultImage(language)
		assert.Equal(t, want, image)
	}
}

func TestIsUpstreamImage(t *testing.T) {
	tests := []struct {
		name  string
		image string
		want  bool
	}{
		{
			name:  "upstream image at the tag we default to",
			image: upstreamImageRepositoryPrefix + "python:" + defaultImageTags[Python],
			want:  true,
		},
		{
			// The tag is deliberately ignored: this answers "is this upstream's
			// image", not "is this upstream's current image".
			name:  "upstream image at an older tag",
			image: upstreamImageRepositoryPrefix + "python:0.50b0",
			want:  true,
		},
		{
			name:  "upstream image by digest",
			image: upstreamImageRepositoryPrefix + "python@sha256:" + "a3f2d1c4e5b60718293a4b5c6d7e8f90a1b2c3d4e5f60718293a4b5c6d7e8f90",
			want:  true,
		},
		{
			// A mirror is not recognized, which is what makes the defaulting
			// annotation the primary signal rather than this test.
			name:  "the same image mirrored into a private registry",
			image: "registry.example.com/mirrors/autoinstrumentation-python:" + defaultImageTags[Python],
			want:  false,
		},
		{
			name:  "another language's upstream image",
			image: upstreamImageRepositoryPrefix + "java:" + defaultImageTags[Java],
			want:  false,
		},
		{
			name:  "an unrelated image",
			image: "registry.example.com/my-own-python-sdk:1",
			want:  false,
		},
		{
			name:  "an unparseable reference",
			image: "NOT A REFERENCE",
			want:  false,
		},
		{
			name:  "no image",
			image: "",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isUpstreamImage(tt.image, Python))
		})
	}
}

func TestLanguageImage(t *testing.T) {
	spec := otelv1alpha1.InstrumentationSpec{
		Java:   otelv1alpha1.Java{Image: "java-image"},
		NodeJS: otelv1alpha1.NodeJS{Image: "nodejs-image"},
		Python: otelv1alpha1.Python{Image: "python-image"},
		DotNet: otelv1alpha1.DotNet{Image: "dotnet-image"},
	}

	assert.Equal(t, "java-image", languageImage(spec, Java))
	assert.Equal(t, "nodejs-image", languageImage(spec, NodeJS))
	assert.Equal(t, "python-image", languageImage(spec, Python))
	assert.Equal(t, "dotnet-image", languageImage(spec, DotNet))
	assert.Empty(t, languageImage(spec, Language("go")))
}

// The annotation keys are upstream's, and a typo in one of them would silently turn
// every mode decision into a heuristic. They are spelled out here rather than built from
// the same expression the implementation uses.
func TestDefaultImageAnnotationKeys(t *testing.T) {
	assert.Equal(t, "instrumentation.opentelemetry.io/default-auto-instrumentation-java-image", Java.defaultImageAnnotation())
	assert.Equal(t, "instrumentation.opentelemetry.io/default-auto-instrumentation-nodejs-image", NodeJS.defaultImageAnnotation())
	assert.Equal(t, "instrumentation.opentelemetry.io/default-auto-instrumentation-python-image", Python.defaultImageAnnotation())
	assert.Equal(t, "instrumentation.opentelemetry.io/default-auto-instrumentation-dotnet-image", DotNet.defaultImageAnnotation())
}
