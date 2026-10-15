// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Imported from https://github.com/aquasecurity/trivy/blob/main/pkg/fanal/image/daemon/image.go

//go:build trivy

package trivy

import (
	"context"
	"time"

	cdx "github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/datadog-agent/pkg/sbom/bomconvert"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/types"
)

const imageCreatedPropertyKey = "datadog:image:Created"

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

	appendSBOMImageCreated(bom, report.Metadata.ImageConfig.Created.Time)

	bom14 := bomconvert.ConvertBOM(bom, opts.simplifyBomRefs)

	return &Report{
		id:  id,
		bom: bom14,
	}, nil
}

// appendSBOMImageCreated records the image build time on the SBOM root
// component.
func appendSBOMImageCreated(bom *cdx.BOM, created time.Time) {
	if created.IsZero() || bom.Metadata == nil || bom.Metadata.Component == nil {
		return
	}
	c := bom.Metadata.Component
	if c.Properties == nil {
		c.Properties = &[]cdx.Property{}
	}
	*c.Properties = append(*c.Properties, cdx.Property{
		Name:  imageCreatedPropertyKey,
		Value: created.UTC().Format(time.RFC3339Nano),
	})
}

// ToCycloneDX returns the report as a CycloneDX SBOM
func (r *Report) ToCycloneDX() *cyclonedx_v1_4.Bom {
	return r.bom
}

// ID returns the report identifier
func (r *Report) ID() string {
	return r.id
}
