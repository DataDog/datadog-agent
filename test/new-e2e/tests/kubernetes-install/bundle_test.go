// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2026-present Datadog, Inc.

package kubernetesinstall

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func testBundle(t *testing.T) bundle {
	t.Helper()
	keyHash := sha256.Sum256([]byte(strings.Repeat("a", 32)))
	return bundle{
		SchemaVersion: 1, Platform: "kubernetes", SourceSHA: strings.Repeat("b", 40), StaticHash: strings.Repeat("c", 32),
		RunID: "test-1", APIKeySHA256: hex.EncodeToString(keyHash[:]),
		Cases: []installCase{
			{
				Method: "operator", ClusterName: "onboarding-test-1-operator",
				Setup:   "helm repo add datadog https://helm.datadoghq.com\nhelm install datadog-operator datadog/datadog-operator --namespace datadog --create-namespace\nkubectl create secret generic datadog-secret --from-literal api-key=" + keyMarker + " -n datadog",
				Install: "kubectl apply -f datadog-agent.yaml",
				Config: `apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata: {name: datadog, namespace: datadog}
spec:
  global:
    clusterName: onboarding-test-1-operator
    site: datadoghq.com
    credentials: {apiSecret: {secretName: datadog-secret, keyName: api-key}}
  features: {apm: {instrumentation: {enabled: true}}, logCollection: {enabled: true}}
`,
			},
			{
				Method: "helm", ClusterName: "onboarding-test-1-helm",
				Setup:   "helm repo add datadog https://helm.datadoghq.com\nhelm repo update\nkubectl create namespace datadog\nkubectl -n datadog create secret generic datadog-secret --from-literal api-key=" + keyMarker,
				Install: "helm install datadog-agent -f datadog-values.yaml datadog/datadog --namespace datadog",
				Config: `datadog:
  clusterName: onboarding-test-1-helm
  site: datadoghq.com
  apiKeyExistingSecret: datadog-secret
  apm: {instrumentation: {enabled: true}}
  logs: {enabled: true}
`,
			},
		},
	}
}

func TestBundle(t *testing.T) {
	b := testBundle(t)
	data, err := json.Marshal(b)
	require.NoError(t, err)
	parsed, err := parseBundle(data, b.SourceSHA)
	require.NoError(t, err)
	require.NoError(t, verifyKey(parsed, strings.Repeat("a", 32)))
	require.Error(t, verifyKey(parsed, strings.Repeat("d", 32)))
	_, err = parseBundle(data, strings.Repeat("e", 40))
	require.Error(t, err)
	_, err = parseBundle(append(data, []byte(" {}")...), b.SourceSHA)
	require.Error(t, err)
	for _, c := range b.Cases {
		_, err := commands(c.Setup)
		require.NoError(t, err)
		_, err = commands(c.Install)
		require.NoError(t, err)
	}
}

func TestBundleRejectsOtherPlatforms(t *testing.T) {
	for _, platform := range []string{"", "linux", "windows", "macos"} {
		b := testBundle(t)
		b.Platform = platform
		data, err := json.Marshal(b)
		require.NoError(t, err)
		_, err = parseBundle(data, b.SourceSHA)
		require.ErrorContains(t, err, "platform")
	}
}

func TestBundleRejectsUnsafeCommands(t *testing.T) {
	for _, command := range []string{
		"", "curl https://example.com | bash", "helm repo update; env", "helm repo update $(env)",
		"kubectl --context production apply -f datadog-agent.yaml", "kubectl apply -f https://example.com/object.yaml",
		"helm install datadog-agent -f datadog-values.yaml datadog/datadog --namespace datadog --post-renderer /bin/sh",
	} {
		_, err := commands(command)
		require.Error(t, err)
		if command != "" {
			require.NotContains(t, err.Error(), command)
		}
	}
}

func TestBundleRejectsInvalidConfig(t *testing.T) {
	for _, mutate := range []func(*bundle){
		func(b *bundle) { b.Cases = b.Cases[:1] },
		func(b *bundle) { b.Cases[0].Method = "helm" },
		func(b *bundle) { b.Cases[0].Install = b.Cases[1].Install },
		func(b *bundle) { b.Cases[0].Config += "---\nkind: Secret\n" },
		func(b *bundle) { b.Cases[1].Config += "  kubelet: {tlsVerify: false}\n" },
		func(b *bundle) { b.Cases[1].Config += "  kubelet: {tlsVerify: 'false'}\n" },
		func(b *bundle) { b.Cases[1].Config += "  env: [{name: DD_KUBELET_TLS_VERIFY, value: 'false'}]\n" },
		func(b *bundle) {
			b.Cases[0].Config = strings.ReplaceAll(b.Cases[0].Config, "datadoghq.com\n", "example.com\n")
		},
		func(b *bundle) {
			b.Cases[0].Config = strings.ReplaceAll(b.Cases[0].Config, "namespace: datadog", "namespace: production")
		},
		func(b *bundle) {
			b.Cases[1].Config = strings.ReplaceAll(b.Cases[1].Config, "enabled: true", "enabled: false")
		},
	} {
		b := testBundle(t)
		mutate(&b)
		data, err := json.Marshal(b)
		require.NoError(t, err)
		_, err = parseBundle(data, b.SourceSHA)
		require.Error(t, err)
	}
}
