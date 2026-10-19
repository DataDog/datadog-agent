// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package sbomutil

import (
	"slices"
	"strconv"
	"strings"
	"time"

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

// UsageObservedSinceProperty names the metadata property of a runtime usage
// report holding when its usage started being recorded, as a unix timestamp.
// MergeRuntimeProperties carries it onto the merged SBOM.
const UsageObservedSinceProperty = "UsageObservedSince"

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

// usageKey returns the key the merge matches a component of a usage report
// by: its name and its version stripped of the epoch.
func usageKey(comp *cyclonedx_v1_4.Component) string {
	version, _ := normalizeVersion(comp.GetVersion())
	return comp.GetName() + "@" + version
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

// HasOSPackage reports whether bom lists an OS package, and so whether the
// runtime scanner reports usage for it.
func HasOSPackage(bom *cyclonedx_v1_4.Bom) bool {
	return slices.ContainsFunc(bom.GetComponents(), isOSPackage)
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
			newComponentsMap[usageKey(comp)] = comp
		}
	}

	// Shallow-copy the BOM envelope; Components is rebuilt below.
	mergedBom := &cyclonedx_v1_4.Bom{
		SpecVersion:        existingBom.SpecVersion,
		Version:            existingBom.Version,
		SerialNumber:       existingBom.SerialNumber,
		Metadata:           carryUsageObservedSince(existingBom.Metadata, newBom.GetMetadata()),
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

		key := usageKey(existingComp)

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

// usageObservedSince returns the property of metadata holding the start of usage
// observation, or nil.
func usageObservedSince(metadata *cyclonedx_v1_4.Metadata) *cyclonedx_v1_4.Property {
	for _, p := range metadata.GetProperties() {
		if p.GetName() == UsageObservedSinceProperty {
			return p
		}
	}
	return nil
}

// carryUsageObservedSince returns the metadata of a merged BOM: that of the
// existing BOM, holding the start of usage observation of the report merged
// into it in place of its own. It leaves both inputs as they are.
func carryUsageObservedSince(existing, report *cyclonedx_v1_4.Metadata) *cyclonedx_v1_4.Metadata {
	since := usageObservedSince(report)
	if since == nil && usageObservedSince(existing) == nil {
		return existing
	}

	merged := &cyclonedx_v1_4.Metadata{
		Timestamp:   existing.GetTimestamp(),
		Tools:       existing.GetTools(),
		Authors:     existing.GetAuthors(),
		Component:   existing.GetComponent(),
		Manufacture: existing.GetManufacture(),
		Supplier:    existing.GetSupplier(),
		Licenses:    existing.GetLicenses(),
	}
	for _, p := range existing.GetProperties() {
		if p.GetName() != UsageObservedSinceProperty {
			merged.Properties = append(merged.Properties, p)
		}
	}
	if since != nil {
		merged.Properties = append(merged.Properties, since)
	}
	return merged
}

// UsageObservedSince returns when the runtime usage bom carries started being
// recorded, and whether bom holds that time.
func UsageObservedSince(bom *cyclonedx_v1_4.Bom) (time.Time, bool) {
	since := usageObservedSince(bom.GetMetadata())
	if since == nil {
		return time.Time{}, false
	}

	seconds, err := strconv.ParseInt(since.GetValue(), 10, 64)
	if err != nil {
		return time.Time{}, false
	}
	return time.Unix(seconds, 0), true
}

// UsageWindowOpen reports whether, at now, the runtime usage bom carries was
// recorded for less than window, the time it takes to tell an unused package
// from one that runs now and then.
func UsageWindowOpen(bom *cyclonedx_v1_4.Bom, now time.Time, window time.Duration) bool {
	since, ok := UsageObservedSince(bom)
	return ok && now.Sub(since) < window
}

// HideUnobserved removes the runtime properties of the components of bom whose
// LastSeenRunning is "0", the packages unseen so far, which leaves their usage
// unknown. It replaces their property slices, which merged components share
// with the components they come from.
func HideUnobserved(bom *cyclonedx_v1_4.Bom) {
	for _, comp := range bom.GetComponents() {
		if unobserved(comp) {
			comp.Properties = slices.DeleteFunc(slices.Clone(comp.Properties), isRuntimeProperty)
		}
	}
}

// HideUnreported removes the runtime properties of the OS packages of bom that
// report lacks, such as a package installed after system-probe last indexed
// the host, which leaves their usage unknown. A report without components
// leaves bom as it is, as the merge does.
func HideUnreported(bom, report *cyclonedx_v1_4.Bom) {
	if len(report.GetComponents()) == 0 {
		return
	}

	reported := make(map[string]struct{}, len(report.GetComponents()))
	for _, comp := range report.GetComponents() {
		reported[usageKey(comp)] = struct{}{}
	}

	for _, comp := range bom.GetComponents() {
		if _, ok := reported[usageKey(comp)]; !ok && isOSPackage(comp) {
			comp.Properties = slices.DeleteFunc(slices.Clone(comp.Properties), isRuntimeProperty)
		}
	}
}

// unobserved reports whether the component carries a LastSeenRunning of "0".
func unobserved(comp *cyclonedx_v1_4.Component) bool {
	for _, p := range comp.GetProperties() {
		if p.GetName() == LastAccessProperty {
			return p.GetValue() == "0"
		}
	}
	return false
}

// isRuntimeProperty reports whether p is one of the runtime usage properties.
func isRuntimeProperty(p *cyclonedx_v1_4.Property) bool {
	switch p.GetName() {
	case LastAccessProperty, HasSetSuidBitProperty, RunningAsRootProperty:
		return true
	}
	return false
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
