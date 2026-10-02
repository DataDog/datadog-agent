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

func findInjectionSource(t *testing.T, mutator *TargetMutator, name injectionSourceName) injectionSource {
	t.Helper()
	for _, entry := range mutator.sources {
		if entry.name == name {
			return entry.source
		}
	}
	require.FailNow(t, "injection source not found", "name: %s", name)
	return nil
}

func TestInjectAllSourceActivation(t *testing.T) {
	const configYAML = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: static
`
	mutator := newMatchMutator(t, configYAML, newMatchTestWmeta(t))

	require.Equal(t, sourcePass, findInjectionSource(t, mutator, injectionSourceInjectAll).resolve(&corev1.Pod{}).action)

	const injectAllConfig = `
apm_config:
  instrumentation:
    enabled: true
`
	injectAllMutator := newMatchMutator(t, injectAllConfig, newMatchTestWmeta(t))
	require.Equal(t, sourceInject, findInjectionSource(t, injectAllMutator, injectionSourceInjectAll).resolve(&corev1.Pod{}).action)

	require.NoError(t, injectAllMutator.SetRemotePolicies([]policies.Policy{{
		Name:    "remote",
		Rules:   policies.AlwaysTrue(),
		Outcome: policies.Outcome{Inject: true, InjectSet: true},
	}}))
	require.Equal(t, sourcePass, findInjectionSource(t, injectAllMutator, injectionSourceInjectAll).resolve(&corev1.Pod{}).action)
	injectAllMutator.ClearRemotePolicies()
	require.Equal(t, sourceInject, findInjectionSource(t, injectAllMutator, injectionSourceInjectAll).resolve(&corev1.Pod{}).action)
}
