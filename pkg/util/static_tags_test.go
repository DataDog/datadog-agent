// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package util

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config"
)

func TestStaticTags(t *testing.T) {
	mockConfig := config.Mock(t)
	mockConfig.Set("kubernetes_kubelet_nodename", "eksnode", config.SourceDefault)
	defer mockConfig.Set("kubernetes_kubelet_nodename", "", config.SourceDefault)

	config.SetFeatures(t, config.EKSFargate)

	t.Run("just tags", func(t *testing.T) {
		mockConfig.Set("tags", []string{"some:tag", "another:tag", "nocolon"}, config.SourceDefault)
		defer mockConfig.Set("tags", []string{}, config.SourceDefault)
		staticTags := GetStaticTags(context.Background())
		assert.Equal(t, map[string]string{
			"some":             "tag",
			"another":          "tag",
			"eks_fargate_node": "eksnode",
		}, staticTags)
	})

	t.Run("tags and extra_tags", func(t *testing.T) {
		mockConfig.Set("tags", []string{"some:tag", "nocolon"}, config.SourceDefault)
		mockConfig.Set("extra_tags", []string{"extra:tag", "missingcolon"}, config.SourceDefault)
		defer mockConfig.Set("tags", []string{}, config.SourceDefault)
		defer mockConfig.Set("extra_tags", []string{}, config.SourceDefault)
		staticTags := GetStaticTags(context.Background())
		assert.Equal(t, map[string]string{
			"some":             "tag",
			"extra":            "tag",
			"eks_fargate_node": "eksnode",
		}, staticTags)
	})

	t.Run("cluster name already set", func(t *testing.T) {
		mockConfig.Set("tags", []string{"kube_cluster_name:foo"}, config.SourceDefault)
		defer mockConfig.Set("tags", []string{}, config.SourceDefault)
		staticTags := GetStaticTags(context.Background())
		assert.Equal(t, map[string]string{
			"eks_fargate_node":  "eksnode",
			"kube_cluster_name": "foo",
		}, staticTags)
	})
}

func TestStaticTagsSlice(t *testing.T) {
	mockConfig := config.Mock(t)
	mockConfig.Set("kubernetes_kubelet_nodename", "eksnode", config.SourceDefault)
	defer mockConfig.Set("kubernetes_kubelet_nodename", "", config.SourceDefault)

	config.SetFeatures(t, config.EKSFargate)

	t.Run("just tags", func(t *testing.T) {
		mockConfig.Set("tags", []string{"some:tag", "another:tag", "nocolon"}, config.SourceDefault)
		defer mockConfig.Set("tags", []string{}, config.SourceDefault)
		staticTags := GetStaticTagsSlice(context.Background())
		assert.ElementsMatch(t, []string{
			"nocolon",
			"some:tag",
			"another:tag",
			"eks_fargate_node:eksnode",
		}, staticTags)
	})

	t.Run("tags and extra_tags", func(t *testing.T) {
		mockConfig.Set("tags", []string{"some:tag", "nocolon"}, config.SourceDefault)
		mockConfig.Set("extra_tags", []string{"extra:tag", "missingcolon"}, config.SourceDefault)
		defer mockConfig.Set("tags", []string{}, config.SourceDefault)
		defer mockConfig.Set("extra_tags", []string{}, config.SourceDefault)
		staticTags := GetStaticTagsSlice(context.Background())
		assert.ElementsMatch(t, []string{
			"nocolon",
			"missingcolon",
			"some:tag",
			"extra:tag",
			"eks_fargate_node:eksnode",
		}, staticTags)
	})
}
