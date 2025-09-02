// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package sbomutil

import (
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
	trivycore "github.com/aquasecurity/trivy/pkg/sbom/core"
	trivydx "github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/stretchr/testify/assert"
)

func TestCompressUncompressSBOM(t *testing.T) {
	sbom := &workloadmeta.SBOM{
		CycloneDXBOM: &cyclonedx_v1_4.Bom{
			Metadata: &cyclonedx_v1_4.Metadata{
				Component: &cyclonedx_v1_4.Component{
					Properties: []*cyclonedx_v1_4.Property{
						{Name: trivydx.Namespace + trivycore.PropertyRepoDigest, Value: pointer.Ptr("digest1")},
						{Name: trivydx.Namespace + trivycore.PropertyRepoTag, Value: pointer.Ptr("tag1")},
					},
				},
			},
		},
		GenerationTime:     time.Now(),
		GenerationDuration: 100 * time.Millisecond,
		Status:             workloadmeta.Success,
		Error:              "",
	}

	csbom, err := CompressSBOM(sbom)
	if err != nil {
		t.Fatal(err)
	}

	usbom, err := UncompressSBOM(csbom)
	if err != nil {
		t.Fatal(err)
	}

	// use EqualExportedValues to compare protobuf values (internal state may be different)
	assert.EqualExportedValues(t, sbom, usbom, "Uncompressed SBOM does not match original")
}
