// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build trivy

// Package util contains utility functions for image metadata collection
package util

import (
	"slices"

	"github.com/CycloneDX/cyclonedx-go"
	trivycore "github.com/aquasecurity/trivy/pkg/sbom/core"
	trivydx "github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/mohae/deepcopy"

	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
)

const (
	repoTagPropertyKey    = trivydx.Namespace + trivycore.PropertyRepoTag
	repoDigestPropertyKey = trivydx.Namespace + trivycore.PropertyRepoDigest
)

// UpdateSBOMRepoMetadata finds if the repo tags and repo digests are present in the SBOM and updates them if not.
// It returns a copy of the SBOM with the updated properties if they were not already present.
func UpdateSBOMRepoMetadata(sbom *workloadmeta.SBOM, repoTags, repoDigests []string) *workloadmeta.SBOM {
	if sbom == nil ||
		sbom.Status != workloadmeta.Success ||
		sbom.CycloneDXBOM == nil ||
		sbom.CycloneDXBOM.Metadata == nil ||
		sbom.CycloneDXBOM.Metadata.Component == nil ||
		sbom.CycloneDXBOM.Metadata.Component.Properties == nil {
		return sbom
	}

	properties := *sbom.CycloneDXBOM.Metadata.Component.Properties
	mismatched := !propertiesEqualsValues(properties, repoTags, repoTagPropertyKey) || !propertiesEqualsValues(properties, repoDigests, repoDigestPropertyKey)

	if mismatched {
		sbom = deepcopy.Copy(sbom).(*workloadmeta.SBOM)
		newProperties := *sbom.CycloneDXBOM.Metadata.Component.Properties
		newProperties = cleanProperties(newProperties)

		newProperties = appendProperties(newProperties, repoTags, repoTagPropertyKey)
		newProperties = appendProperties(newProperties, repoDigests, repoDigestPropertyKey)

		sbom.CycloneDXBOM.Metadata.Component.Properties = &newProperties
	}

	return sbom
}

// propertiesEqualsValues function compares the existing properties with the new values
func propertiesEqualsValues(properties []cyclonedx.Property, newValues []string, propertyKeyType string) bool {
	existingValuesMap := make(map[string]struct{})
	for _, prop := range properties {
		if prop.Name == propertyKeyType {
			existingValuesMap[prop.Value] = struct{}{}
		}
	}

	for _, newValue := range newValues {
		if _, found := existingValuesMap[newValue]; !found {
			return false
		}
	}

	for existingValue := range existingValuesMap {
		if !slices.Contains(newValues, existingValue) {
			return false
		}
	}

	return true
}

// Remove properties from the list that are present in the mismatched map
func cleanProperties(properties []cyclonedx.Property) []cyclonedx.Property {
	res := make([]cyclonedx.Property, 0, len(properties))

	for _, prop := range properties {
		if prop.Name == repoTagPropertyKey || prop.Name == repoDigestPropertyKey {
			continue
		}
		res = append(res, prop)
	}

	return res
}

// Append new values to the properties that were not already present
func appendProperties(properties []cyclonedx.Property, newValues []string, propertyKeyType string) []cyclonedx.Property {
	existingValues := make(map[string]struct{})
	for _, prop := range properties {
		if prop.Name == propertyKeyType {
			existingValues[prop.Value] = struct{}{}
		}
	}

	for _, newValue := range newValues {
		if _, found := existingValues[newValue]; !found {
			prop := cdxProperty(propertyKeyType, newValue)
			properties = append(properties, prop)
		}
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
