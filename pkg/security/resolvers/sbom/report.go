// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package sbom

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	cyclonedx_v1_4 "github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// PackagesReport wraps package data and implements the sbom.Report interface
type PackagesReport struct {
	packages    []sbomtypes.PackageWithInstalledFiles
	containerID containerutils.ContainerID
}

// NewPackagesReport creates a new PackagesReport from a slice of packages
func NewPackagesReport(packages []sbomtypes.PackageWithInstalledFiles, containerID containerutils.ContainerID) *PackagesReport {
	return &PackagesReport{
		packages:    packages,
		containerID: containerID,
	}
}

// ToCycloneDX converts the packages to a CycloneDX BOM with LastAccess properties
func (r *PackagesReport) ToCycloneDX() *cyclonedx_v1_4.Bom {
	components := make([]*cyclonedx_v1_4.Component, 0, len(r.packages))

	for _, pkg := range r.packages {
		purl := "pkg:" + pkg.Name + "@" + pkg.Version
		if pkg.Release != "" {
			purl += "-" + pkg.Release
		}

		component := &cyclonedx_v1_4.Component{
			Type:    cyclonedx_v1_4.Classification_CLASSIFICATION_LIBRARY,
			Name:    pkg.Name,
			Version: pkg.Version,
			Purl:    pointer.Ptr(purl),
		}

		// Add LastAccess property if available
		if !pkg.LastAccess.IsZero() {
			lastAccess := pkg.LastAccess.Format(time.RFC3339)
			component.Properties = []*cyclonedx_v1_4.Property{
				{
					Name:  "LastAccess",
					Value: pointer.Ptr(lastAccess),
				},
			}
		}

		components = append(components, component)
	}

	return &cyclonedx_v1_4.Bom{
		Components: components,
	}
}

// ID returns a unique identifier for this report
func (r *PackagesReport) ID() string {
	// Generate ID from container ID
	hash := sha256.Sum256([]byte(r.containerID))
	return hex.EncodeToString(hash[:])
}
