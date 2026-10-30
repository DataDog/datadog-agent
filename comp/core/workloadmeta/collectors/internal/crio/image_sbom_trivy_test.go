// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build crio && trivy

package crio

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/sbom"
)

// TestProcessScanResultDropsDeletedImage checks that the collector drops the
// SBOM of an image deleted during its scan, which its Set event would recreate.
func TestProcessScanResultDropsDeletedImage(t *testing.T) {
	image := func(id string) *workloadmeta.ContainerImageMetadata {
		return &workloadmeta.ContainerImageMetadata{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainerImageMetadata, ID: id}}
	}
	store := &mockWorkloadmetaStore{existingImages: map[string]*workloadmeta.ContainerImageMetadata{"present": image("present")}}
	c := collector{store: store}

	for _, id := range []string{"present", "deleted"} {
		c.processScanResult(sbom.ScanResult{ImgMeta: image(id), Error: errors.New("scan error")})
	}

	require.Len(t, store.notifiedEvents, 1)
	assert.Equal(t, "present", store.notifiedEvents[0].Entity.GetID().ID)
}
