// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Imported from https://github.com/aquasecurity/trivy/blob/main/pkg/fanal/image/daemon/image.go

//go:build trivy

package trivy

import (
	cyclonedxgo "github.com/CycloneDX/cyclonedx-go"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/types"
)

// Report describes a trivy report along with its marshaler
type Report struct {
	*types.Report
	id        string
	marshaler *cyclonedx.Marshaler
}

// ToCycloneDX returns the report as a CycloneDX SBOM
func (r *Report) ToCycloneDX() (*cyclonedxgo.BOM, error) {
	bom, err := r.marshaler.Marshal(*r.Report)
	if err != nil {
		return nil, err
	}

	// We don't need the dependencies attribute. Remove to save memory.
	bom.Dependencies = nil
	return bom, nil
}

// ID returns the report identifier
func (r *Report) ID() string {
	return r.id
}
