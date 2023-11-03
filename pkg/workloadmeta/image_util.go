// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build trivy

package workloadmeta

import (
	"github.com/CycloneDX/cyclonedx-go"
	trivydx "github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
)

const (
	repoTagPropertyKey    = trivydx.Namespace + trivydx.PropertyRepoTag
	repoDigestPropertyKey = trivydx.Namespace + trivydx.PropertyRepoDigest
)

// updateSBOMRepoMetadata updates entered SBOM with new metadata properties if the initial SBOM status was successful
// and there are new repoTags and repoDigests missing in the SBOM. It returns the updated SBOM.
func updateSBOMRepoMetadata(sbom *SBOM, repoTags, repoDigests []string) *SBOM {
	if sbom.Status != Success || sbom.CycloneDXBOM.Metadata.Component.Properties == nil {
		return sbom
	}

	properties := *sbom.CycloneDXBOM.Metadata.Component.Properties
	properties = removeOldRepoProperties(properties)

	properties = appendMissingProperties(properties, repoTags, repoTagPropertyKey)
	properties = appendMissingProperties(properties, repoDigests, repoDigestPropertyKey)

	sbom.CycloneDXBOM.Metadata.Component.Properties = &properties

	return sbom
}

// removeOldRepoProperties returns an array without repodigests and repoTags
func removeOldRepoProperties(properties []cyclonedx.Property) []cyclonedx.Property {
	res := make([]cyclonedx.Property, 0, len(properties))

	for _, prop := range properties {
		if prop.Name == repoTagPropertyKey || prop.Name == repoDigestPropertyKey {
			continue
		}
		res = append(res, prop)
	}

	return res
}

// appendMissingProperties function updates the list of properties along with the property set,
// if the given repoValues are not already present in the set.
func appendMissingProperties(properties []cyclonedx.Property, repoValues []string, propertyKeyType string) []cyclonedx.Property {
	for _, repoValue := range repoValues {
		prop := cdxProperty(propertyKeyType, repoValue)
		properties = append(properties, prop)
	}
	return properties
}

// cdxProperty function generates a trivy-specific cycloneDX Property.
func cdxProperty(key, value string) cyclonedx.Property {
	return cyclonedx.Property{
		Name:  key,
		Value: value,
	}
}
