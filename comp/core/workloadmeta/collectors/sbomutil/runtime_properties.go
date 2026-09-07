// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sbomutil

import (
	"slices"
	"strings"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Names of the runtime usage properties ("package in use") that the CWS SBOM
// resolver reports for a package and that MergeRuntimeProperties carries onto a
// Trivy SBOM.
const (
	LastAccessProperty    = "LastSeenRunning"
	HasSetSuidBitProperty = "HasSetSuidBit"
	RunningAsRootProperty = "RunningAsRoot"
)

// normalizeVersion normalizes version strings to handle epoch differences
// e.g., "1:4.4.36-4build1" and "4.4.36-4build1" should both map to "4.4.36-4build1"
// Returns both the normalized version (without epoch) and the original version
func normalizeVersion(version string) (normalized string, hasEpoch bool) {
	// Check if version has epoch prefix (e.g., "1:4.4.36-4build1")
	if idx := strings.Index(version, ":"); idx > 0 {
		// Extract the part after the epoch
		return version[idx+1:], true
	}
	return version, false
}

// osPackagePurlPrefixes are the purl types of the packages the runtime scanner
// observes. It reads the dpkg, rpm and apk databases, see
// pkg/security/resolvers/sbom/collectorv2.NewOSScanner, and Trivy maps every OS
// family onto one of these three types.
var osPackagePurlPrefixes = []string{"pkg:deb/", "pkg:rpm/", "pkg:apk/"}

// isOSPackage reports whether the component is an OS package, and so something
// the runtime scanner could have seen running.
func isOSPackage(comp *cyclonedx_v1_4.Component) bool {
	for _, prefix := range osPackagePurlPrefixes {
		if strings.HasPrefix(comp.GetPurl(), prefix) {
			return true
		}
	}
	return false
}

// hasForeignPurl reports whether the component's purl places it outside the
// databases the runtime scanner reads. A component carrying no purl is left to
// the name and version match.
func hasForeignPurl(comp *cyclonedx_v1_4.Component) bool {
	return comp.GetPurl() != "" && !isOSPackage(comp)
}

// MergeRuntimeProperties merges runtime properties from newBom into existingBom.
// Returns a new BOM whose component list is deduplicated (by bom-ref) and enriched with
// runtime properties (LastSeenRunning / HasSetSuidBit / RunningAsRoot) taken from newBom.
// The properties are defaulted on the OS packages, the ones in the scanner's scope, so
// that their absence elsewhere reads as "out of scope".  Deduplication guards against a
// stored image SBOM carrying the same component twice, and the bom-ref is what identifies
// a component: Trivy reports one entry per install location, so one library version
// legitimately appears once per lockfile that pins it.
func MergeRuntimeProperties(existingBom, newBom *cyclonedx_v1_4.Bom) *cyclonedx_v1_4.Bom {
	if newBom == nil || len(newBom.Components) == 0 {
		return existingBom
	}

	// Build a lookup map from newBom (system-probe) components by name+normalised version.
	// We normalise versions to handle epoch differences (e.g. "1:4.4.36" vs "4.4.36").
	newComponentsMap := make(map[string]*cyclonedx_v1_4.Component)
	for _, comp := range newBom.Components {
		if comp != nil {
			normalizedVersion, _ := normalizeVersion(comp.Version)
			key := comp.Name + "@" + normalizedVersion
			newComponentsMap[key] = comp
		}
	}

	// Shallow-copy the BOM envelope; Components is rebuilt below.
	mergedBom := &cyclonedx_v1_4.Bom{
		SpecVersion:        existingBom.SpecVersion,
		Version:            existingBom.Version,
		SerialNumber:       existingBom.SerialNumber,
		Metadata:           existingBom.Metadata,
		Services:           existingBom.Services,
		ExternalReferences: existingBom.ExternalReferences,
		Dependencies:       existingBom.Dependencies,
		Compositions:       existingBom.Compositions,
		Vulnerabilities:    existingBom.Vulnerabilities,
	}

	// seen tracks the bom-refs already emitted, so a component appearing twice is
	// emitted once. Deduplication applies to the components that carry a bom-ref,
	// and the others are emitted in turn.
	seen := make(map[string]struct{}, len(existingBom.Components))

	for _, existingComp := range existingBom.Components {
		if existingComp == nil {
			continue
		}

		if ref := existingComp.GetBomRef(); ref != "" {
			if _, already := seen[ref]; already {
				continue
			}
			seen[ref] = struct{}{}
		}

		normalizedVersion, _ := normalizeVersion(existingComp.Version)
		key := existingComp.Name + "@" + normalizedVersion

		// Copy all fields so we do not mutate the original BOM. The property
		// slice is cloned because updateProperty replaces entries in place.
		mergedComp := &cyclonedx_v1_4.Component{
			Type:               existingComp.Type,
			MimeType:           existingComp.MimeType,
			BomRef:             existingComp.BomRef,
			Supplier:           existingComp.Supplier,
			Author:             existingComp.Author,
			Publisher:          existingComp.Publisher,
			Group:              existingComp.Group,
			Name:               existingComp.Name,
			Version:            existingComp.Version,
			Description:        existingComp.Description,
			Scope:              existingComp.Scope,
			Hashes:             existingComp.Hashes,
			Licenses:           existingComp.Licenses,
			Copyright:          existingComp.Copyright,
			Cpe:                existingComp.Cpe,
			Purl:               existingComp.Purl,
			Swid:               existingComp.Swid,
			Modified:           existingComp.Modified,
			Pedigree:           existingComp.Pedigree,
			ExternalReferences: existingComp.ExternalReferences,
			Components:         existingComp.Components,
			Properties:         slices.Clone(existingComp.Properties),
			Evidence:           existingComp.Evidence,
			ReleaseNotes:       existingComp.ReleaseNotes,
		}

		// Add or update runtime properties from newBom. The report describes the
		// dpkg, rpm and apk databases, so a component whose purl sits elsewhere
		// shares a name and a version with it and nothing more.
		newComp, reported := newComponentsMap[key]
		if reported && hasForeignPurl(mergedComp) {
			reported = false
		}
		if reported && newComp.Properties != nil {
			updateProperty := func(propertyName string) {
				var newProp *cyclonedx_v1_4.Property
				for _, prop := range newComp.Properties {
					if prop != nil && prop.Name == propertyName {
						newProp = prop
						break
					}
				}
				if newProp == nil {
					return
				}
				if mergedComp.Properties == nil {
					mergedComp.Properties = []*cyclonedx_v1_4.Property{}
				}
				for j, prop := range mergedComp.Properties {
					if prop != nil && prop.Name == propertyName {
						mergedComp.Properties[j] = newProp
						log.Tracef("Updated %s for component %s@%s", propertyName, existingComp.Name, existingComp.Version)
						return
					}
				}
				mergedComp.Properties = append(mergedComp.Properties, newProp)
				log.Tracef("Added %s for component %s@%s", propertyName, existingComp.Name, existingComp.Version)
			}

			updateProperty(LastAccessProperty)
			updateProperty(HasSetSuidBitProperty)
			updateProperty(RunningAsRootProperty)
		}

		// Default the runtime properties on what the scanner covers: the OS
		// packages, and whatever the report carried. Elsewhere the absence of a
		// property marks the component out of the scanner's scope.
		if reported || isOSPackage(mergedComp) {
			ensureProperty(mergedComp, LastAccessProperty, "0")
			ensureProperty(mergedComp, HasSetSuidBitProperty, "false")
			ensureProperty(mergedComp, RunningAsRootProperty, "false")
		}

		mergedBom.Components = append(mergedBom.Components, mergedComp)
	}

	return mergedBom
}

// ensureProperty appends a property with the given name and value to the
// component unless it already carries one with that name.
func ensureProperty(comp *cyclonedx_v1_4.Component, name, value string) {
	for _, p := range comp.Properties {
		if p != nil && p.Name == name {
			return
		}
	}
	v := value
	comp.Properties = append(comp.Properties, &cyclonedx_v1_4.Property{Name: name, Value: &v})
}
