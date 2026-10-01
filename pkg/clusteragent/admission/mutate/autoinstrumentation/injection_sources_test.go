// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/dd-policy-engine/go/policies"
)

type stubInjectionSource struct {
	result sourceResult
}

func (s stubInjectionSource) resolve(*corev1.Pod) sourceResult {
	return s.result
}

func mutatorWithSources(entries ...injectionSourceEntry) *TargetMutator {
	return &TargetMutator{sources: entries}
}

func injectPlanResult(name string) sourceResult {
	return sourceResult{action: sourceInject, plan: &injectionPlan{name: name}}
}

func TestTargetMutatorSourcePrecedence(t *testing.T) {
	tests := []struct {
		name            string
		entries         []injectionSourceEntry
		wantPlanName    string
		wantSource      injectionSourceName
		wantSSI         bool
		wantNoSelection bool
	}{
		{
			name: "annotation selects target while RC determines SSI mode",
			entries: []injectionSourceEntry{
				{name: injectionSourceAnnotation, source: stubInjectionSource{injectPlanResult("annotation")}},
				{name: injectionSourceDatadogInstrumentation, determinesSSIMode: true, source: stubInjectionSource{sourceResult{action: sourcePass}}},
				{name: injectionSourceRemoteConfig, determinesSSIMode: true, source: stubInjectionSource{injectPlanResult("remote")}},
				{name: injectionSourceStatic, determinesSSIMode: true, source: stubInjectionSource{injectPlanResult("static")}},
			},
			wantPlanName: "annotation",
			wantSource:   injectionSourceAnnotation,
			wantSSI:      true,
		},
		{
			name: "DDI wins over RC and static",
			entries: []injectionSourceEntry{
				{name: injectionSourceAnnotation, source: stubInjectionSource{sourceResult{action: sourcePass}}},
				{name: injectionSourceDatadogInstrumentation, determinesSSIMode: true, source: stubInjectionSource{injectPlanResult("ddi")}},
				{name: injectionSourceRemoteConfig, determinesSSIMode: true, source: stubInjectionSource{injectPlanResult("remote")}},
				{name: injectionSourceStatic, determinesSSIMode: true, source: stubInjectionSource{injectPlanResult("static")}},
			},
			wantPlanName: "ddi",
			wantSource:   injectionSourceDatadogInstrumentation,
			wantSSI:      true,
		},
		{
			name: "RC wins over GPU and static",
			entries: []injectionSourceEntry{
				{name: injectionSourceAnnotation, source: stubInjectionSource{sourceResult{action: sourcePass}}},
				{name: injectionSourceDatadogInstrumentation, determinesSSIMode: true, source: stubInjectionSource{sourceResult{action: sourcePass}}},
				{name: injectionSourceRemoteConfig, determinesSSIMode: true, source: stubInjectionSource{injectPlanResult("remote")}},
				{name: injectionSourceGPU, determinesSSIMode: true, source: stubInjectionSource{injectPlanResult("gpu")}},
				{name: injectionSourceStatic, determinesSSIMode: true, source: stubInjectionSource{injectPlanResult("static")}},
			},
			wantPlanName: "remote",
			wantSource:   injectionSourceRemoteConfig,
			wantSSI:      true,
		},
		{
			name: "RC pass falls through to GPU before static",
			entries: []injectionSourceEntry{
				{name: injectionSourceRemoteConfig, determinesSSIMode: true, source: stubInjectionSource{sourceResult{action: sourcePass}}},
				{name: injectionSourceGPU, determinesSSIMode: true, source: stubInjectionSource{injectPlanResult("gpu")}},
				{name: injectionSourceStatic, determinesSSIMode: true, source: stubInjectionSource{injectPlanResult("static")}},
			},
			wantPlanName: "gpu",
			wantSource:   injectionSourceGPU,
			wantSSI:      true,
		},
		{
			name: "RC denial blocks static",
			entries: []injectionSourceEntry{
				{name: injectionSourceRemoteConfig, determinesSSIMode: true, source: stubInjectionSource{sourceResult{action: sourceDeny}}},
				{name: injectionSourceStatic, determinesSSIMode: true, source: stubInjectionSource{injectPlanResult("static")}},
			},
			wantNoSelection: true,
		},
		{
			name: "annotation remains selected when SSI source denies",
			entries: []injectionSourceEntry{
				{name: injectionSourceAnnotation, source: stubInjectionSource{injectPlanResult("annotation")}},
				{name: injectionSourceRemoteConfig, determinesSSIMode: true, source: stubInjectionSource{sourceResult{action: sourceDeny}}},
				{name: injectionSourceStatic, determinesSSIMode: true, source: stubInjectionSource{injectPlanResult("static")}},
			},
			wantPlanName: "annotation",
			wantSource:   injectionSourceAnnotation,
			wantSSI:      false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolved := mutatorWithSources(test.entries...).getTarget(&corev1.Pod{})
			if test.wantNoSelection {
				require.Nil(t, resolved)
				return
			}
			require.NotNil(t, resolved)
			require.Equal(t, test.wantPlanName, resolved.plan.name)
			require.Equal(t, test.wantSource, resolved.selectedBy)
			require.Equal(t, test.wantSSI, resolved.isSSI)
		})
	}
}

func TestTargetMutatorSourceChain(t *testing.T) {
	const configYAML = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: static
`
	mutator := newMatchMutator(t, configYAML, newMatchTestWmeta(t))

	sourceNames := func(mutator *TargetMutator) []injectionSourceName {
		names := make([]injectionSourceName, 0, len(mutator.sources))
		for _, entry := range mutator.sources {
			names = append(names, entry.name)
		}
		return names
	}

	wantSources := []injectionSourceName{
		injectionSourceAnnotation,
		injectionSourceDatadogInstrumentation,
		injectionSourceRemoteConfig,
		injectionSourceGPU,
		injectionSourceStatic,
		injectionSourceInjectAll,
	}
	require.Equal(t, wantSources, sourceNames(mutator))
	require.Equal(t, sourcePass, mutator.injectAllSource.resolve(&corev1.Pod{}).action)

	require.NoError(t, mutator.SetRemotePolicies([]policies.Policy{{
		Name:    "remote",
		Rules:   policies.AlwaysTrue(),
		Outcome: policies.Outcome{Inject: true, InjectSet: true},
	}}))
	require.Equal(t, wantSources, sourceNames(mutator))

	const injectAllConfig = `
apm_config:
  instrumentation:
    enabled: true
`
	injectAllMutator := newMatchMutator(t, injectAllConfig, newMatchTestWmeta(t))
	require.Equal(t, wantSources, sourceNames(injectAllMutator))
	require.Equal(t, sourcePass, injectAllMutator.remoteSource.resolve(&corev1.Pod{}).action)
	require.Equal(t, sourcePass, injectAllMutator.staticSource.resolve(&corev1.Pod{}).action)
	require.Equal(t, sourceInject, injectAllMutator.injectAllSource.resolve(&corev1.Pod{}).action)

	require.NoError(t, injectAllMutator.SetRemotePolicies([]policies.Policy{{
		Name:    "remote",
		Rules:   policies.AlwaysTrue(),
		Outcome: policies.Outcome{Inject: true, InjectSet: true},
	}}))
	require.Equal(t, sourcePass, injectAllMutator.injectAllSource.resolve(&corev1.Pod{}).action)
	injectAllMutator.ClearRemotePolicies()
	require.Equal(t, sourceInject, injectAllMutator.injectAllSource.resolve(&corev1.Pod{}).action)
}
