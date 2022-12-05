// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build containerd
// +build containerd

package containerd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func TestUpdateContainerImageMetadata(t *testing.T) {
	tests := []struct {
		name                         string
		imageMetadata                workloadmeta.ContainerImageMetadata
		newName                      string
		expectedUpdatedImageMetadata workloadmeta.ContainerImageMetadata
		expectsUpdate                bool
	}{
		{
			name: "new name is a digest",
			imageMetadata: workloadmeta.ContainerImageMetadata{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainerImageMetadata,
					ID:   "sha256:12345",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: "gcr.io/datadoghq/cluster-agent:7.40.1",
				},
				ShortName: "cluster-agent",
				RepoTags: []string{
					"gcr.io/datadoghq/cluster-agent:7.40.1",
				},
			},
			newName:       "sha256:123",
			expectsUpdate: false,
		},
		{
			name: "new name replaces old and is added as repo tag",
			imageMetadata: workloadmeta.ContainerImageMetadata{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainerImageMetadata,
					ID:   "sha256:12345",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: "sha256:12345",
				},
				ShortName: "",
				RepoTags:  []string{},
			},
			newName: "gcr.io/datadoghq/cluster-agent:7.40.1",
			expectedUpdatedImageMetadata: workloadmeta.ContainerImageMetadata{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainerImageMetadata,
					ID:   "sha256:12345",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name: "gcr.io/datadoghq/cluster-agent:7.40.1",
				},
				ShortName: "cluster-agent",
				RepoTags: []string{
					"gcr.io/datadoghq/cluster-agent:7.40.1",
				},
			},
			expectsUpdate: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testCollector := collector{}

			changed := testCollector.updateContainerImageMetadata(&test.imageMetadata, test.newName)

			if test.expectsUpdate {
				assert.True(t, changed)
				assert.Equal(t, test.expectedUpdatedImageMetadata, test.imageMetadata)
			} else {
				assert.False(t, changed)
			}
		})
	}
}
