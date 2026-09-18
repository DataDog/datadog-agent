// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package otelinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLanguageAnnotationKeys(t *testing.T) {
	tests := []struct {
		language       Language
		wantInject     string
		wantContainers string
	}{
		{Java, "instrumentation.opentelemetry.io/inject-java", "instrumentation.opentelemetry.io/java-container-names"},
		{NodeJS, "instrumentation.opentelemetry.io/inject-nodejs", "instrumentation.opentelemetry.io/nodejs-container-names"},
		{Python, "instrumentation.opentelemetry.io/inject-python", "instrumentation.opentelemetry.io/python-container-names"},
		{DotNet, "instrumentation.opentelemetry.io/inject-dotnet", "instrumentation.opentelemetry.io/dotnet-container-names"},
	}

	for _, tt := range tests {
		t.Run(string(tt.language), func(t *testing.T) {
			assert.Equal(t, tt.wantInject, tt.language.injectAnnotation())
			assert.Equal(t, tt.wantContainers, tt.language.containerNamesAnnotation())
		})
	}
}

func TestParseInjectValue(t *testing.T) {
	tests := []struct {
		name          string
		value         string
		wantDirective injectDirective
		wantNamespace string
		wantName      string
	}{
		{
			name:          "absent",
			value:         "",
			wantDirective: directiveNone,
		},
		{
			name:          "false",
			value:         "false",
			wantDirective: directiveDisabled,
		},
		{
			name:          "false is case insensitive",
			value:         "FaLsE",
			wantDirective: directiveDisabled,
		},
		{
			name:          "true",
			value:         "true",
			wantDirective: directiveSoleInNamespace,
		},
		{
			name:          "true is case insensitive",
			value:         "TRUE",
			wantDirective: directiveSoleInNamespace,
		},
		{
			name:          "bare name resolves in the pod namespace",
			value:         "my-instrumentation",
			wantDirective: directiveNamed,
			wantNamespace: "pod-ns",
			wantName:      "my-instrumentation",
		},
		{
			name:          "qualified name",
			value:         "other-ns/my-instrumentation",
			wantDirective: directiveNamed,
			wantNamespace: "other-ns",
			wantName:      "my-instrumentation",
		},
		{
			name:          "split is on the first slash only",
			value:         "other-ns/nested/name",
			wantDirective: directiveNamed,
			wantNamespace: "other-ns",
			wantName:      "nested/name",
		},
		{
			name:          "empty namespace before the slash is kept verbatim",
			value:         "/my-instrumentation",
			wantDirective: directiveNamed,
			wantNamespace: "",
			wantName:      "my-instrumentation",
		},
		{
			// Upstream treats anything that is not true/false as a name, so these end up
			// as lookups that simply miss.
			name:          "unrecognised value is a name, not an enable",
			value:         "yes",
			wantDirective: directiveNamed,
			wantNamespace: "pod-ns",
			wantName:      "yes",
		},
		{
			name:          "numeric value is a name",
			value:         "1",
			wantDirective: directiveNamed,
			wantNamespace: "pod-ns",
			wantName:      "1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseInjectValue(tt.value, "pod-ns")
			assert.Equal(t, tt.wantDirective, got.directive)
			assert.Equal(t, tt.wantNamespace, got.namespace)
			assert.Equal(t, tt.wantName, got.name)
		})
	}
}

// TestEffectiveAnnotationValue is the table from the upstream reference document,
// which mirrors upstream's own TestEffectiveAnnotationValue.
func TestEffectiveAnnotationValue(t *testing.T) {
	const key = "instrumentation.opentelemetry.io/inject-java"

	tests := []struct {
		name          string
		podValue      string
		nsValue       string
		want          string
		wantRationale string
	}{
		{
			name:          "both absent",
			want:          "",
			wantRationale: "no injection",
		},
		{
			name:          "pod absent, namespace true",
			nsValue:       "true",
			want:          "true",
			wantRationale: "namespace applies to all its pods",
		},
		{
			name:          "pod absent, namespace names a CR",
			nsValue:       "ns-cr",
			want:          "ns-cr",
			wantRationale: "namespace applies to all its pods",
		},
		{
			name:          "pod absent, namespace false",
			nsValue:       "false",
			want:          "false",
			wantRationale: "namespace applies to all its pods",
		},
		{
			name:          "pod true, namespace absent",
			podValue:      "true",
			want:          "true",
			wantRationale: "pod value used as-is",
		},
		{
			name:          "pod false beats namespace true",
			podValue:      "false",
			nsValue:       "true",
			want:          "false",
			wantRationale: "opt-out is always honoured",
		},
		{
			name:          "pod false beats a namespace named CR",
			podValue:      "false",
			nsValue:       "ns-cr",
			want:          "false",
			wantRationale: "opt-out is always honoured",
		},
		{
			name:          "pod named CR beats namespace true",
			podValue:      "pod-cr",
			nsValue:       "true",
			want:          "pod-cr",
			wantRationale: "an explicit pod choice is final",
		},
		{
			name:          "pod named CR beats a namespace named CR",
			podValue:      "pod-cr",
			nsValue:       "ns-cr",
			want:          "pod-cr",
			wantRationale: "an explicit pod choice is final",
		},
		{
			name:          "pod true beats namespace false",
			podValue:      "true",
			nsValue:       "false",
			want:          "true",
			wantRationale: "the pod re-enables what the namespace disabled",
		},
		{
			// The counter-intuitive one: "true" on the pod only means "yes, instrument
			// me" and delegates the choice of CR to the namespace.
			name:          "namespace named CR beats pod true",
			podValue:      "true",
			nsValue:       "ns-cr",
			want:          "ns-cr",
			wantRationale: "pod 'true' delegates the choice of CR",
		},
		{
			name:          "both true",
			podValue:      "true",
			nsValue:       "true",
			want:          "true",
			wantRationale: "",
		},
		{
			name:          "pod TRUE still delegates to the namespace",
			podValue:      "TRUE",
			nsValue:       "ns-cr",
			want:          "ns-cr",
			wantRationale: "comparisons are case-insensitive",
		},
		{
			name:          "pod true beats namespace FALSE",
			podValue:      "true",
			nsValue:       "FALSE",
			want:          "true",
			wantRationale: "comparisons are case-insensitive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			podAnnotations := map[string]string{}
			if tt.podValue != "" {
				podAnnotations[key] = tt.podValue
			}
			nsAnnotations := map[string]string{}
			if tt.nsValue != "" {
				nsAnnotations[key] = tt.nsValue
			}

			got := effectiveAnnotationValue(podAnnotations, nsAnnotations, key)
			assert.Equal(t, tt.want, got, tt.wantRationale)
		})
	}
}

func TestEffectiveAnnotationValueWithNilMaps(t *testing.T) {
	const key = "instrumentation.opentelemetry.io/inject-java"

	// A nil namespace map is how resolution degrades when no NamespaceAnnotationGetter
	// is wired: the pod value must simply win.
	assert.Equal(t, "true", effectiveAnnotationValue(map[string]string{key: "true"}, nil, key))
	assert.Equal(t, "ns-cr", effectiveAnnotationValue(nil, map[string]string{key: "ns-cr"}, key))
	assert.Equal(t, "", effectiveAnnotationValue(nil, nil, key))
}

func TestParseContainerNames(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    []string
		wantErr bool
	}{
		{
			name:  "empty selects nothing and is not an error",
			value: "",
			want:  nil,
		},
		{
			name:  "single name",
			value: "app",
			want:  []string{"app"},
		},
		{
			name:  "comma separated list",
			value: "app,sidecar",
			want:  []string{"app", "sidecar"},
		},
		{
			name:  "hyphens and digits are allowed",
			value: "my-app-1,other-2",
			want:  []string{"my-app-1", "other-2"},
		},
		{
			name:  "uppercase is allowed by the pattern",
			value: "App",
			want:  []string{"App"},
		},
		{
			// Upstream's ^[a-zA-Z0-9-,]+$ rejects dots even though they are legal
			// elsewhere in Kubernetes.
			name:    "dots are rejected",
			value:   "my.app",
			wantErr: true,
		},
		{
			name:    "underscores are rejected",
			value:   "my_app",
			wantErr: true,
		},
		{
			name:    "spaces are rejected",
			value:   "app, sidecar",
			wantErr: true,
		},
		{
			name:    "slashes are rejected",
			value:   "ns/app",
			wantErr: true,
		},
		{
			// A comma is in the allowed character class, so upstream accepts this and
			// ends up with two empty container names that match nothing.
			name:  "a lone comma passes validation and yields empty names",
			value: ",",
			want:  []string{"", ""},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseContainerNames(tt.value)
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMappableLanguagesOrderMatchesUpstream(t *testing.T) {
	// The order decides which language's failure is reported when several would fail,
	// because upstream aborts the pod on the first one.
	assert.Equal(t, []Language{Java, NodeJS, Python, DotNet}, MappableLanguages)
}
