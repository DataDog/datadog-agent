// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product contains software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubelet && kubeapiserver

package hostinfo

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Test partitions:
// - annotation shape: reserved prefix | reserved prefix with empty key | non-reserved | no annotations
// - value mapping: one tag per annotation, deterministic order

// TestExtractTagRuleTagsReservedPrefix covers: reserved-prefix annotations map
// to tag-rule tags; non-reserved annotations are ignored; order is sorted.
func TestExtractTagRuleTagsReservedPrefix(t *testing.T) {
	annotations := map[string]string{
		"tags.datadoghq.com/tag.owning_team":        "frontend",
		"tags.datadoghq.com/tag.is_leader":          "true",
		"cluster.k8s.io/machine":                    "ben-bitdiddle-machine",
		"ad.datadoghq.com/pod-leader-election.tags": "{}",
	}

	tags := extractTagRuleTags(annotations)
	assert.Equal(t, []string{"is_leader:true", "owning_team:frontend"}, tags)
}

// TestExtractTagRuleTagsEdgeCases covers: empty annotation map; empty tag key
// after the reserved prefix is skipped; a bare prefix annotation is skipped.
func TestExtractTagRuleTagsEdgeCases(t *testing.T) {
	assert.Nil(t, extractTagRuleTags(nil))
	assert.Nil(t, extractTagRuleTags(map[string]string{}))
	assert.Nil(t, extractTagRuleTags(map[string]string{
		"tags.datadoghq.com/tag.":      "eva-lu-ator",
		"node.alpha.kubernetes.io/ttl": "0",
	}))
}

// TestExtractTagRuleTagsValueNormalization covers: values pass through
// unchanged; an empty value still produces the tag with an empty value,
// since the tag-rule controller owns the value domain.
func TestExtractTagRuleTagsValueNormalization(t *testing.T) {
	tags := extractTagRuleTags(map[string]string{
		"tags.datadoghq.com/tag.is_schedulable": "false",
		"tags.datadoghq.com/tag.empty_value":    "",
	})
	assert.Equal(t, []string{"empty_value:", "is_schedulable:false"}, tags)
}
