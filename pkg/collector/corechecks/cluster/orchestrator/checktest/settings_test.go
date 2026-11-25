// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package checktest

import (
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

// TestSettings checks the settings that change the payloads of every resource. Each test states how its payloads
// differ from the test data files.
func TestSettings(t *testing.T) {
	t.Run("container_scrubbing_disabled", func(t *testing.T) {
		env := suite.Setup(t, suite.WithSetting("orchestrator_explorer.container_scrubbing.enabled", false))

		expected := suite.ReadPayloads(t, contentObjects(t)...).Walk(envValue("PASSWORD", "secret"))
		actual := env.Pop(t, expected.Objects()...)
		suite.AssertPayloads(t, expected, actual)
	})

	t.Run("custom_sensitive_words", func(t *testing.T) {
		env := suite.Setup(t, suite.WithSetting("orchestrator_explorer.custom_sensitive_words", []string{"region"}))

		expected := suite.ReadPayloads(t, contentObjects(t)...).Walk(envValue("REGION", "********"))
		actual := env.Pop(t, expected.Objects()...)
		suite.AssertPayloads(t, expected, actual)
	})

	t.Run("custom_sensitive_annotations_labels", func(t *testing.T) {
		env := suite.Setup(t, suite.WithSetting("orchestrator_explorer.custom_sensitive_annotations_labels", []string{"app"}))

		expected := suite.ReadPayloads(t, contentObjects(t)...).Walk(labelValue("app", "-"))
		// The Secret informer drops sensitive annotations and labels, instead of redacting their values.
		secret := suite.Payloads{"live v1_secrets": expected["live v1_secrets"]}.Walk(withoutLabel("app"))
		expected["live v1_secrets"] = secret["live v1_secrets"]
		actual := env.Pop(t, expected.Objects()...)
		suite.AssertPayloads(t, expected, actual)
	})

	// A family of built-in custom resources that its setting disables is not collected.
	t.Run("service_mesh_custom_resources_disabled", func(t *testing.T) {
		env := suite.Setup(t, suite.WithSetting("orchestrator_explorer.custom_resources.ootb.service_mesh", false))
		var serviceMesh []string
		for _, group := range []string{
			"networking.istio.io", "gateway.envoyproxy.io", "traefik.containo.us", "policy.linkerd.io", "consul.hashicorp.com",
			"mesh.consul.hashicorp.com", "kuma.io",
		} {
			serviceMesh = append(serviceMesh, withPrefix(env.UIDs(), group+"_")...)
		}
		expected := firstRun(except(env.UIDs(), serviceMesh...))

		suite.AssertObjects(t, expected, env.Pop(t, expected...))
	})
}

// Helm releases are ConfigMaps with the owner=helm label, which the Helm release collector sends with their data when
// its setting is on. Without it, the default, the ConfigMap collector sends every ConfigMap, without its data.
func TestHelmReleases(t *testing.T) {
	objects := []string{"live v1_configmaps", "live v1_helm-releases-configmaps"}

	t.Run("with_the_helm_release_collector", func(t *testing.T) {
		env := suite.Setup(t, suite.WithCollectors("configmaps"))

		actual := env.Pop(t, objects...)
		suite.AssertObjects(t, objects, actual)
		suite.AssertPayloads(t, suite.ReadPayloads(t, objects...), actual)
	})

	t.Run("without_the_helm_release_collector", func(t *testing.T) {
		env := suite.Setup(t,
			suite.WithSetting("orchestrator_explorer.helm_releases.configmaps.enabled", false),
			suite.WithCollectors("configmaps"))

		// The ConfigMap collector sends the metadata of a ConfigMap only, so the release has no data.
		expected := suite.ReadPayloads(t, objects...)
		release := suite.Payloads{"live v1_helm-releases-configmaps": expected["live v1_helm-releases-configmaps"]}.Walk(withoutContentField("data"))
		expected["live v1_helm-releases-configmaps"] = release["live v1_helm-releases-configmaps"]
		actual := env.Pop(t, objects...)
		suite.AssertObjects(t, objects, actual)
		suite.AssertPayloads(t, expected, actual)
	})
}

// withoutContentField returns a Walk function that removes a field from the content of a manifest.
func withoutContentField(name string) func(string, any) any {
	return func(key string, field any) any {
		if content, ok := field.(map[string]any); ok && key == "content" {
			delete(content, name)
		}
		return field
	}
}

// withoutLabel returns a Walk function that removes the label from a manifest.
func withoutLabel(name string) func(string, any) any {
	return func(key string, field any) any {
		if labels, ok := field.(map[string]any); ok && key == "labels" {
			delete(labels, name)
		}
		return field
	}
}

// envValue returns a Walk function that sets the value of the environment variable.
func envValue(name, value string) func(string, any) any {
	return func(key string, field any) any {
		if variable, ok := field.(map[string]any); ok && key == "env" && variable["name"] == name {
			variable["value"] = value
		}
		return field
	}
}

// labelValue returns a Walk function that sets the value of the label, in a manifest and in a metadata model, which
// lists labels as key:value.
func labelValue(name, value string) func(string, any) any {
	return func(key string, field any) any {
		switch labels := field.(type) {
		case map[string]any:
			if key == "labels" && labels[name] != nil {
				labels[name] = value
			}
		case []any:
			if key == "labels" {
				for i, label := range labels {
					if s, ok := label.(string); ok && strings.HasPrefix(s, name+":") {
						labels[i] = name + ":" + value
					}
				}
			}
		}
		return field
	}
}
