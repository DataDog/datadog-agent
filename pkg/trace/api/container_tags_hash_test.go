// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

// The container tags hash returned in the Datadog-Container-Tags-Hash header
// is folded into the tracers' Data Streams Monitoring (DSM) and Database
// Monitoring (DBM) base hash. Every distinct value creates new pathways in the
// DSM backend, so any tag added to serviceOriginTags must be low cardinality
// and stable across rolling deploys, restarts and job runs.
//
// This file is owned by @DataDog/data-streams-monitoring (see CODEOWNERS). If
// you need to change these tests, please reach out to #data-streams-monitoring
// first.

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestServiceOriginTagsAllowlist pins the exact set of tags allowed into the
// container tags hash.
func TestServiceOriginTagsAllowlist(t *testing.T) {
	expected := map[string]struct{}{
		"kube_deployment":     {},
		"kube_cronjob":        {},
		"kube_container_name": {},
		"kube_namespace":      {},
		"kube_app_name":       {},
		"kube_app_managed_by": {},
		"service":             {},
		"short_image":         {},
		"kube_cluster_name":   {},
	}
	assert.Equal(t, expected, serviceOriginTags, "serviceOriginTags changed: this affects DSM pathway hash cardinality, please get a review from @DataDog/data-streams-monitoring")
}

// TestContainerTagsHashExcludesHighCardinalityTags makes sure tags that change
// per pod, container, image build or job run never reach the hash.
func TestContainerTagsHashExcludesHighCardinalityTags(t *testing.T) {
	highCardinalityTags := []string{
		"container_id",
		"container_name",
		"pod_name",
		"pod_uid",
		"pod_phase",
		"kube_replica_set",
		"kube_job",
		"kube_ownerref_name",
		"image_id",
		"image_tag",
		"git.commit.sha",
		"host",
		"task_arn",
	}
	for _, tag := range highCardinalityTags {
		t.Run(tag, func(t *testing.T) {
			assert.NotContains(t, serviceOriginTags, tag)
			assert.Equal(t, computeContainerTagsHash(nil), computeContainerTagsHash([]string{tag + ":value"}))
		})
	}
}

func TestComputeContainerTagsHash(t *testing.T) {
	stableTags := []string{
		"kube_cluster_name:clusterA",
		"kube_namespace:namespace1",
		"kube_container_name:app",
		"service:svc",
	}
	hashOf := func(tags ...string) string {
		return fmt.Sprintf("%x", sha256.Sum256([]byte(strings.Join(tags, ","))))
	}

	t.Run("only service origin tags are hashed", func(t *testing.T) {
		tags := append([]string{"container_id:abc", "pod_name:app-5d8f7c6b9-x2x7z", "image_tag:v1"}, stableTags...)
		expected := hashOf("kube_cluster_name:clusterA", "kube_container_name:app", "kube_namespace:namespace1", "service:svc")
		assert.Equal(t, expected, computeContainerTagsHash(tags))
	})

	// The hash must not change on every rolling deploy or job run, but it
	// should still tell workloads apart.
	t.Run("stable across rolling deploys", func(t *testing.T) {
		before := append([]string{"kube_deployment:app", "kube_replica_set:app-5d8f7c6b9"}, stableTags...)
		after := append([]string{"kube_deployment:app", "kube_replica_set:app-7f4b9d8c5"}, stableTags...)
		expected := hashOf("kube_cluster_name:clusterA", "kube_container_name:app", "kube_deployment:app", "kube_namespace:namespace1", "service:svc")
		assert.Equal(t, expected, computeContainerTagsHash(before))
		assert.Equal(t, expected, computeContainerTagsHash(after))
	})

	t.Run("stable across job runs", func(t *testing.T) {
		before := append([]string{"kube_cronjob:report", "kube_job:report-29012340"}, stableTags...)
		after := append([]string{"kube_cronjob:report", "kube_job:report-29012400"}, stableTags...)
		expected := hashOf("kube_cluster_name:clusterA", "kube_container_name:app", "kube_cronjob:report", "kube_namespace:namespace1", "service:svc")
		assert.Equal(t, expected, computeContainerTagsHash(before))
		assert.Equal(t, expected, computeContainerTagsHash(after))
	})

	t.Run("stable regardless of tag order", func(t *testing.T) {
		reversed := []string{"service:svc", "kube_container_name:app", "kube_namespace:namespace1", "kube_cluster_name:clusterA"}
		assert.Equal(t, computeContainerTagsHash(stableTags), computeContainerTagsHash(reversed))
	})

	t.Run("different deployments hash differently", func(t *testing.T) {
		canary := append([]string{"kube_deployment:app-canary", "kube_replica_set:app-canary-5d8f7c6b9"}, stableTags...)
		stable := append([]string{"kube_deployment:app", "kube_replica_set:app-5d8f7c6b9"}, stableTags...)
		assert.NotEqual(t, computeContainerTagsHash(canary), computeContainerTagsHash(stable))
	})
}
