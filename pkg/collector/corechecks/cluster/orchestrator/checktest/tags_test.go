// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package checktest

import (
	"slices"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/checktest/suite"
)

func TestTags(t *testing.T) {
	// The infrastructure mode tags of a cluster check runner come from the tagger stream of the Cluster Agent, which
	// can connect after the check is configured. The check reads them at each run, and adds them to its payloads.
	t.Run("infra_tags_after_configure", func(t *testing.T) {
		env := suite.Setup(t, suite.WithMode(suite.ClusterCheckRunner))
		env.SetInfraTags("infra_mode:cloud_cost_only")

		expected := suite.ReadPayloads(t, contentObjects(t)...).WithMessageTags("infra_mode:cloud_cost_only")
		actual := env.Pop(t, expected.Objects()...)
		suite.AssertPayloads(t, expected, actual)
	})

	// The check adds the tags of its configuration and the global tags of the Agent to every payload.
	sources := []struct {
		name   string
		option suite.Option
		tag    string
	}{
		// A dispatched cluster check carries the tags of its configuration.
		{name: "instance_tags", option: suite.WithInstanceTags("team:checks"), tag: "team:checks"},
		{name: "init_config_tags", option: suite.WithInitConfig("tags:\n  - source:init_config\n"), tag: "source:init_config"},
		// The tagger reports the tags of the Agent configuration, like orchestrator_explorer.extra_tags.
		{name: "global_tags", option: suite.WithGlobalTags("env:global"), tag: "env:global"},
	}
	for _, source := range sources {
		t.Run(source.name, func(t *testing.T) {
			env := suite.Setup(t, source.option)

			expected := suite.ReadPayloads(t, contentObjects(t)...).WithMessageTags(source.tag)
			actual := env.Pop(t, expected.Objects()...)
			suite.AssertPayloads(t, expected, actual)
		})
	}

	// The check only adds the global tags of low cardinality.
	t.Run("global_tags_of_other_cardinalities", func(t *testing.T) {
		env := suite.Setup(t, suite.WithGlobalTagsByCardinality(
			[]string{"cardinality:low"}, []string{"cardinality:orchestrator"}, []string{"cardinality:high"}, []string{"cardinality:standard"}))

		expected := suite.ReadPayloads(t, contentObjects(t)...).WithMessageTags("cardinality:low")
		actual := env.Pop(t, expected.Objects()...)
		suite.AssertPayloads(t, expected, actual)
	})

	// The processors add the tags that the tagger reports for an object to its metadata model, whose tags its manifest
	// repeats. The pod processor reads them from the pod tag provider instead, and the cluster has no entity.
	t.Run("entity_tags", func(t *testing.T) {
		env := suite.Setup(t)
		env.SetEntityTags("entity:tag")

		expected := suite.ReadPayloads(t, contentObjects(t)...)
		for object, payloads := range expected {
			if !hasMetadataModel(payloads) || object == "live v1_pods" || object == "live cluster" {
				continue
			}
			for _, payload := range payloads {
				model := payload["model"].(map[string]any)
				tags, _ := model["tags"].([]any)
				model["tags"] = sortedTags(append(tags, "entity:tag"))
			}
		}
		actual := env.Pop(t, expected.Objects()...)
		suite.AssertPayloads(t, expected, actual)
	})
}

func sortedTags(tags []any) []any {
	return slices.SortedFunc(slices.Values(tags), func(a, b any) int { return strings.Compare(a.(string), b.(string)) })
}

func hasMetadataModel(payloads []map[string]any) bool {
	return slices.ContainsFunc(payloads, func(p map[string]any) bool { return p["type"] == "metadata" })
}
