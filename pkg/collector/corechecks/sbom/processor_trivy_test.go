// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build trivy

package sbom

import (
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/sbomutil"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// TestImageSBOMListsCurrentRepoNames checks that an image SBOM goes out with
// the repo tags and digests the image has, though the SBOM of the store, the
// copy its runtime usage merged into, lists those of an earlier time.
func TestImageSBOMListsCurrentRepoNames(t *testing.T) {
	const (
		repoTagProperty    = "aquasecurity:trivy:RepoTag"
		repoDigestProperty = "aquasecurity:trivy:RepoDigest"
	)
	p, store, sent := newImageGraceProcessor(t)

	img := graceImage(t, workloadmeta.Success)
	scan := &cyclonedx_v1_4.Bom{
		Metadata: &cyclonedx_v1_4.Metadata{Component: &cyclonedx_v1_4.Component{Properties: []*cyclonedx_v1_4.Property{
			{Name: repoTagProperty, Value: pointer.Ptr(img.RepoTags[0])},
			{Name: repoDigestProperty, Value: pointer.Ptr(img.RepoDigests[0])},
		}}},
		Components: shellPackages(),
	}
	since := time.Unix(1690000000, 0)
	img.SBOM = mustCompressSBOM(t, &workloadmeta.SBOM{
		CycloneDXBOM:   sbomutil.MergeRuntimeProperties(scan, usageReport(since)),
		GenerationTime: time.Unix(1700000000, 0),
		Status:         workloadmeta.Success,
	})
	img.RepoTags = append(img.RepoTags, "datadog/agent:latest")

	notify(p, store, workloadmeta.Event{Type: workloadmeta.EventTypeSet, Entity: img})
	bom := receiveSBOM(t, sent).GetCyclonedx()
	properties := func(name string) []string {
		var values []string
		for _, prop := range bom.GetMetadata().GetComponent().GetProperties() {
			if prop.GetName() == name {
				values = append(values, prop.GetValue())
			}
		}
		return values
	}
	assert.ElementsMatch(t, img.RepoTags, properties(repoTagProperty), "repo tags of the SBOM")
	assert.ElementsMatch(t, img.RepoDigests, properties(repoDigestProperty), "repo digests of the SBOM")
	assert.Equal(t, "1700000000", lastSeenRunning(bom, "bash"), "usage of bash")
	observed, ok := sbomutil.UsageObservedSince(bom)
	assert.True(t, ok, "the SBOM holds the start of its usage")
	assert.Equal(t, since.Unix(), observed.Unix(), "start of the usage of the SBOM")
}
