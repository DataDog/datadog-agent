// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package profile holds profile related files
package profile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cgroupModel "github.com/DataDog/datadog-agent/pkg/security/resolvers/cgroup/model"
)

// TestProfileStoresTagsOfEveryTrackedImageTag checks that a profile shared by several
// image tags keeps the tags of each of them. AddTags, used for the container that
// creates the profile, keeps a single value per tag name; the tags of the image tags
// tracked afterwards are merged on top of it.
func TestProfileStoresTagsOfEveryTrackedImageTag(t *testing.T) {
	p := New(WithWorkloadSelector(cgroupModel.WorkloadSelector{Image: "app", Tag: "*"}))

	p.AddTags([]string{"container_id:c1", "image_name:app", "image_tag:v1"})
	p.AddVersionContext("v1", &VersionContext{Tags: []string{"container_id:c1", "image_name:app", "image_tag:v1"}})
	p.AddVersionContext("v2", &VersionContext{Tags: []string{"container_id:c2", "image_name:app", "image_tag:v2"}})

	assert.ElementsMatch(t, []string{
		"container_id:c1",
		"image_name:app",
		"image_tag:v1",
		"container_id:c2",
		"image_tag:v2",
	}, p.GetTags())

	// tracking the same image tag again must not duplicate its tags
	p.AddVersionContext("v2", &VersionContext{Tags: []string{"container_id:c2", "image_name:app", "image_tag:v2"}})
	assert.ElementsMatch(t, []string{
		"container_id:c1",
		"image_name:app",
		"image_tag:v1",
		"container_id:c2",
		"image_tag:v2",
	}, p.GetTags())
}

// TestPrepareNewVersionMergesImageTagTags checks the legacy path that learns a new image tag.
func TestPrepareNewVersionMergesImageTagTags(t *testing.T) {
	p := New(WithWorkloadSelector(cgroupModel.WorkloadSelector{Image: "app", Tag: "*"}))

	require.Empty(t, p.PrepareNewVersion("v1", []string{"container_id:c1", "image_name:app", "image_tag:v1"}, 20, 1))
	require.Empty(t, p.PrepareNewVersion("v2", []string{"container_id:c2", "image_name:app", "image_tag:v2"}, 20, 2))

	assert.ElementsMatch(t, []string{
		"container_id:c1",
		"image_name:app",
		"image_tag:v1",
		"container_id:c2",
		"image_tag:v2",
	}, p.GetTags())
}
