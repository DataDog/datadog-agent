// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build trivy

package trivy

import (
	"time"

	cdx "github.com/CycloneDX/cyclonedx-go"
	v1 "github.com/google/go-containerregistry/pkg/v1"
)

const imageCreatedPropertyKey = "datadog:image:Created"

func propertyExists(props []cdx.Property, name string) bool {
	for _, prop := range props {
		if prop.Name == name {
			return true
		}
	}
	return false
}

// appendSBOMImageCreated records the image build time on the SBOM root
// component.
func appendSBOMImageCreated(sbom *cdx.BOM, config *v1.ConfigFile) {
	if config == nil || config.Created.IsZero() ||
		sbom == nil || sbom.Metadata == nil || sbom.Metadata.Component == nil {
		return
	}
	props := sbom.Metadata.Component.Properties
	if props == nil {
		props = &[]cdx.Property{}
	}
	if !propertyExists(*props, imageCreatedPropertyKey) {
		*props = append(*props, cdx.Property{
			Name:  imageCreatedPropertyKey,
			Value: config.Created.UTC().Format(time.RFC3339Nano),
		})
	}
	sbom.Metadata.Component.Properties = props
}
