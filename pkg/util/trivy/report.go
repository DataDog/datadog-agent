// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Imported from https://github.com/aquasecurity/trivy/blob/main/pkg/fanal/image/daemon/image.go

//go:build trivy

package trivy

import (
	"context"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/datadog-agent/pkg/sbom/bomconvert"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/types"
)

// Report describes a trivy report along with its marshaler
type Report struct {
	id  string
	bom *cyclonedx_v1_4.Bom
}

type reportOptions struct {
	dependencies    bool
	simplifyBomRefs bool
}

func newReport(id string, report *types.Report, marshaler cyclonedx.Marshaler, opts reportOptions) (*Report, error) {
	bom, err := marshaler.MarshalReport(context.TODO(), *report)
	if err != nil {
		return nil, err
	}

	if !opts.dependencies {
		bom.Dependencies = nil
	}

	bom14 := bomconvert.ConvertBOM(bom, opts.simplifyBomRefs)

	return &Report{
		id:  id,
		bom: bom14,
	}, nil
}

// ToCycloneDX returns the report as a CycloneDX SBOM
func (r *Report) ToCycloneDX() *cyclonedx_v1_4.Bom {
	return r.bom
}

// ID returns the report identifier
func (r *Report) ID() string {
	return r.id
}
