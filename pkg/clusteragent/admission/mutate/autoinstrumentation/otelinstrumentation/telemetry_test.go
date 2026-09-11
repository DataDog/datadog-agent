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

	telemetry "github.com/DataDog/datadog-agent/comp/core/telemetry/def"
)

// recordingCounter is a telemetry.Counter that only remembers the tags it was
// incremented with.
type recordingCounter struct {
	telemetry.Counter
	increments [][]string
}

func (c *recordingCounter) Inc(tagsValue ...string) {
	c.increments = append(c.increments, tagsValue)
}

// newRecordingResolver builds a resolver whose unresolvable counter can be inspected.
func newRecordingResolver(crs ...*otelv1alpha1.Instrumentation) (*Resolver, *recordingCounter) {
	counter := &recordingCounter{}
	resolver := &Resolver{
		store:     &fakeLookup{crs: crs},
		telemetry: &resolverTelemetry{unresolvable: counter},
	}
	return resolver, counter
}

func TestUnresolvableIsCounted(t *testing.T) {
	tests := []struct {
		name        string
		crs         []*otelv1alpha1.Instrumentation
		annotations map[string]string
		wantTags    []string
	}{
		{
			name:        "no custom resource in the namespace",
			annotations: map[string]string{injectKey(Java): "true"},
			wantTags:    []string{string(ReasonNoInstrumentationInNamespace), "java"},
		},
		{
			name:        "several custom resources in the namespace",
			crs:         []*otelv1alpha1.Instrumentation{newCR("app-ns", "a"), newCR("app-ns", "b")},
			annotations: map[string]string{injectKey(Python): "true"},
			wantTags:    []string{string(ReasonMultipleInstrumentations), "python"},
		},
		{
			name:        "named custom resource missing",
			annotations: map[string]string{injectKey(NodeJS): "nope"},
			wantTags:    []string{string(ReasonInstrumentationNotFound), "nodejs"},
		},
		{
			name: "invalid per-language container names",
			crs:  []*otelv1alpha1.Instrumentation{newCR("app-ns", "shared")},
			annotations: map[string]string{
				injectKey(DotNet):                 "shared",
				DotNet.containerNamesAnnotation(): "my_app",
			},
			wantTags: []string{string(ReasonInvalidContainerNames), "dotnet"},
		},
		{
			// The common annotation belongs to no single language, so the language tag
			// carries a placeholder rather than being empty.
			name: "invalid common container names",
			crs:  []*otelv1alpha1.Instrumentation{newCR("app-ns", "shared")},
			annotations: map[string]string{
				injectKey(Java):                "shared",
				commonContainerNamesAnnotation: "my_app",
			},
			wantTags: []string{string(ReasonInvalidContainerNames), "-"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, counter := newRecordingResolver(tt.crs...)

			result := resolver.Resolve(newPod(tt.annotations), "app-ns")

			require.Equal(t, OutcomeUnresolvable, result.Outcome)
			require.Len(t, counter.increments, 1, "a refusal to inject must not be silent")
			assert.Equal(t, tt.wantTags, counter.increments[0])
		})
	}
}

func TestStoreUnavailableIsCounted(t *testing.T) {
	counter := &recordingCounter{}
	resolver := &Resolver{
		store:     &fakeLookup{notServing: true},
		telemetry: &resolverTelemetry{unresolvable: counter},
	}

	result := resolver.Resolve(newPod(map[string]string{injectKey(Java): "true"}), "app-ns")

	require.Equal(t, OutcomeUnresolvable, result.Outcome)
	require.Len(t, counter.increments, 1)
	assert.Equal(t, []string{string(ReasonStoreUnavailable), "java"}, counter.increments[0])
}

func TestResolvableOutcomesAreNotCounted(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		wantOutcome Outcome
	}{
		{
			name:        "no annotation",
			annotations: nil,
			wantOutcome: OutcomeNoAnnotation,
		},
		{
			name:        "disabled",
			annotations: map[string]string{injectKey(Java): "false"},
			wantOutcome: OutcomeNoAnnotation,
		},
		{
			name:        "resolved",
			annotations: map[string]string{injectKey(Java): "shared"},
			wantOutcome: OutcomeResolved,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, counter := newRecordingResolver(newCR("app-ns", "shared"))

			result := resolver.Resolve(newPod(tt.annotations), "app-ns")

			require.Equal(t, tt.wantOutcome, result.Outcome)
			assert.Empty(t, counter.increments, "only a refusal to inject is counted")
		})
	}
}

// TestResolverTelemetryIsNilSafe guards the resolver against a caller that builds a
// Resolver without going through NewResolver.
func TestResolverTelemetryIsNilSafe(t *testing.T) {
	resolver := &Resolver{store: &fakeLookup{}}

	result := resolver.Resolve(newPod(map[string]string{injectKey(Java): "true"}), "app-ns")

	assert.Equal(t, OutcomeUnresolvable, result.Outcome)
}

func TestNewResolverWiresTelemetry(t *testing.T) {
	// The real counter comes from the shared admission_webhooks subsystem, so this only
	// checks that resolution through the constructor works end to end.
	resolver := NewResolver(NewStore(newFakeClient()), nil, ModeDatadog)
	require.NotNil(t, resolver.telemetry)

	result := resolver.Resolve(newPod(map[string]string{injectKey(Java): "true"}), "app-ns")
	assert.Equal(t, OutcomeUnresolvable, result.Outcome)
	assert.Equal(t, ReasonStoreUnavailable, result.Reason)
}
