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

type stubTargetSource struct {
	result sourceResult
}

func (s stubTargetSource) resolve(*corev1.Pod) sourceResult {
	return s.result
}

func mutatorWithSources(entries ...sourceEntry) *TargetMutator {
	return &TargetMutator{sources: entries}
}

func injectResult(name string) sourceResult {
	return sourceResult{action: sourceInject, target: &targetInternal{name: name}}
}

func TestTargetMutatorSourcePrecedence(t *testing.T) {
	tests := []struct {
		name            string
		entries         []sourceEntry
		wantTarget      string
		wantSource      targetSourceName
		wantSSI         bool
		wantSSISource   targetSourceName
		wantNoSelection bool
	}{
		{
			name: "annotation selects target while RC determines SSI mode",
			entries: []sourceEntry{
				{name: targetSourceAnnotation, source: stubTargetSource{injectResult("annotation")}},
				{name: targetSourceDatadogInstrumentation, determineSSIMode: true, source: stubTargetSource{sourceResult{action: sourceAbstain}}},
				{name: targetSourceRemoteConfig, determineSSIMode: true, source: stubTargetSource{injectResult("remote")}},
				{name: targetSourceStatic, determineSSIMode: true, source: stubTargetSource{injectResult("static")}},
			},
			wantTarget:    "annotation",
			wantSource:    targetSourceAnnotation,
			wantSSI:       true,
			wantSSISource: targetSourceRemoteConfig,
		},
		{
			name: "DDI wins over RC and static",
			entries: []sourceEntry{
				{name: targetSourceAnnotation, source: stubTargetSource{sourceResult{action: sourceAbstain}}},
				{name: targetSourceDatadogInstrumentation, determineSSIMode: true, source: stubTargetSource{injectResult("ddi")}},
				{name: targetSourceRemoteConfig, determineSSIMode: true, source: stubTargetSource{injectResult("remote")}},
				{name: targetSourceStatic, determineSSIMode: true, source: stubTargetSource{injectResult("static")}},
			},
			wantTarget:    "ddi",
			wantSource:    targetSourceDatadogInstrumentation,
			wantSSI:       true,
			wantSSISource: targetSourceDatadogInstrumentation,
		},
		{
			name: "RC wins over static",
			entries: []sourceEntry{
				{name: targetSourceAnnotation, source: stubTargetSource{sourceResult{action: sourceAbstain}}},
				{name: targetSourceDatadogInstrumentation, determineSSIMode: true, source: stubTargetSource{sourceResult{action: sourceAbstain}}},
				{name: targetSourceRemoteConfig, determineSSIMode: true, source: stubTargetSource{injectResult("remote")}},
				{name: targetSourceStatic, determineSSIMode: true, source: stubTargetSource{injectResult("static")}},
			},
			wantTarget:    "remote",
			wantSource:    targetSourceRemoteConfig,
			wantSSI:       true,
			wantSSISource: targetSourceRemoteConfig,
		},
		{
			name: "RC abstention falls through to static",
			entries: []sourceEntry{
				{name: targetSourceRemoteConfig, determineSSIMode: true, source: stubTargetSource{sourceResult{action: sourceAbstain}}},
				{name: targetSourceStatic, determineSSIMode: true, source: stubTargetSource{injectResult("static")}},
			},
			wantTarget:    "static",
			wantSource:    targetSourceStatic,
			wantSSI:       true,
			wantSSISource: targetSourceStatic,
		},
		{
			name: "RC denial blocks static",
			entries: []sourceEntry{
				{name: targetSourceRemoteConfig, determineSSIMode: true, source: stubTargetSource{sourceResult{action: sourceDeny}}},
				{name: targetSourceStatic, determineSSIMode: true, source: stubTargetSource{injectResult("static")}},
			},
			wantNoSelection: true,
		},
		{
			name: "annotation remains selected when SSI source denies",
			entries: []sourceEntry{
				{name: targetSourceAnnotation, source: stubTargetSource{injectResult("annotation")}},
				{name: targetSourceRemoteConfig, determineSSIMode: true, source: stubTargetSource{sourceResult{action: sourceDeny}}},
				{name: targetSourceStatic, determineSSIMode: true, source: stubTargetSource{injectResult("static")}},
			},
			wantTarget:    "annotation",
			wantSource:    targetSourceAnnotation,
			wantSSI:       false,
			wantSSISource: targetSourceRemoteConfig,
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
			require.Equal(t, test.wantTarget, resolved.target.name)
			require.Equal(t, test.wantSource, resolved.selectionSource)
			require.Equal(t, test.wantSSI, resolved.isSSI)
			require.Equal(t, test.wantSSISource, resolved.ssiSource)
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

	sourceNames := func(mutator *TargetMutator) []targetSourceName {
		names := make([]targetSourceName, 0, len(mutator.sources))
		for _, entry := range mutator.sources {
			names = append(names, entry.name)
		}
		return names
	}

	wantSources := []targetSourceName{
		targetSourceAnnotation,
		targetSourceDatadogInstrumentation,
		targetSourceRemoteConfig,
		targetSourceStatic,
		targetSourceInjectAll,
	}
	require.Equal(t, wantSources, sourceNames(mutator))
	require.Equal(t, sourceAbstain, mutator.injectAllSource.resolve(&corev1.Pod{}).action)

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
	require.Equal(t, sourceAbstain, injectAllMutator.remoteSource.resolve(&corev1.Pod{}).action)
	require.Equal(t, sourceAbstain, injectAllMutator.staticSource.resolve(&corev1.Pod{}).action)
	require.Equal(t, sourceInject, injectAllMutator.injectAllSource.resolve(&corev1.Pod{}).action)

	require.NoError(t, injectAllMutator.SetRemotePolicies([]policies.Policy{{
		Name:    "remote",
		Rules:   policies.AlwaysTrue(),
		Outcome: policies.Outcome{Inject: true, InjectSet: true},
	}}))
	require.Equal(t, sourceAbstain, injectAllMutator.injectAllSource.resolve(&corev1.Pod{}).action)
	injectAllMutator.ClearRemotePolicies()
	require.Equal(t, sourceInject, injectAllMutator.injectAllSource.resolve(&corev1.Pod{}).action)
}
