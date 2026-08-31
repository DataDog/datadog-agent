// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package sbom holds sbom related files
package sbom

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	cyclonedx_v1_4 "github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	sbomtypes "github.com/DataDog/datadog-agent/pkg/security/resolvers/sbom/types"
	"github.com/DataDog/datadog-agent/pkg/security/secl/containerutils"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const (
	LastAccessProperty    = "LastSeenRunning"
	HasSetSuidBitProperty = "HasSetSuidBit"
	RunningAsRootProperty = "RunningAsRoot"

	// hostReportID seeds the identifier of the host report, which has no
	// container ID to derive one from.
	hostReportID = "host"
)

// PackagesReport wraps package data and implements the sbom.Report interface
type PackagesReport struct {
	packages    []sbomtypes.Package
	containerID containerutils.ContainerID
	host        bool
}

// NewPackagesReport creates a new PackagesReport for the given container
func NewPackagesReport(packages []sbomtypes.Package, containerID containerutils.ContainerID) *PackagesReport {
	return &PackagesReport{
		packages:    packages,
		containerID: containerID,
	}
}

// NewHostPackagesReport creates a new PackagesReport for the host. The core agent
// identifies the host by its own hostname, so the report carries no id of its own.
func NewHostPackagesReport(packages []sbomtypes.Package) *PackagesReport {
	return &PackagesReport{
		packages: packages,
		host:     true,
	}
}

// IsHost reports whether this report describes the host rather than a container.
func (r *PackagesReport) IsHost() bool {
	return r.host
}

// ToCycloneDX converts the packages to a CycloneDX BOM with LastAccess properties
func (r *PackagesReport) ToCycloneDX() *cyclonedx_v1_4.Bom {
	components := make([]*cyclonedx_v1_4.Component, 0, len(r.packages))

	for _, pkg := range r.packages {
		// Construct version string with epoch, matching Debian/RPM format: [epoch:]version[-release]
		version := ""
		if pkg.Epoch > 0 {
			version = strconv.Itoa(pkg.Epoch) + ":"
		}
		version += pkg.Version
		if pkg.Release != "" {
			version += "-" + pkg.Release
		}

		purl := "pkg:" + pkg.Name + "@" + version

		component := &cyclonedx_v1_4.Component{
			Type:    cyclonedx_v1_4.Classification_CLASSIFICATION_LIBRARY,
			Name:    pkg.Name,
			Version: version,
			Purl:    pointer.Ptr(purl),
		}

		// Always emit LastSeenRunning, "0" meaning never seen running. The
		// core-agent merge overwrites only the runtime properties present in the
		// forwarded report, so omitting a zero value would leave a stale timestamp
		// in place when a package-database refresh resets a package's usage.
		lastAccess := "0"
		if !pkg.LastAccess.IsZero() {
			lastAccess = strconv.FormatInt(pkg.LastAccess.Unix(), 10)
		}
		component.Properties = append(component.Properties, &cyclonedx_v1_4.Property{
			Name:  LastAccessProperty,
			Value: pointer.Ptr(lastAccess),
		})

		suidBit := strconv.FormatBool(pkg.SuidBit)
		component.Properties = append(component.Properties, &cyclonedx_v1_4.Property{
			Name:  HasSetSuidBitProperty,
			Value: pointer.Ptr(suidBit),
		})

		runningAsRoot := strconv.FormatBool(pkg.AccessedByRoot)
		component.Properties = append(component.Properties, &cyclonedx_v1_4.Property{
			Name:  RunningAsRootProperty,
			Value: pointer.Ptr(runningAsRoot),
		})

		components = append(components, component)
	}

	return &cyclonedx_v1_4.Bom{
		Components: components,
	}
}

// ID returns a unique identifier for this report
func (r *PackagesReport) ID() string {
	if r.host {
		hash := sha256.Sum256([]byte(hostReportID))
		return hex.EncodeToString(hash[:])
	}
	// Generate ID from container ID
	hash := sha256.Sum256([]byte(r.containerID))
	return hex.EncodeToString(hash[:])
}
