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
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/bomconvert"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	"github.com/aquasecurity/trivy/pkg/types"
)

const imageCreatedPropertyKey = "datadog:image:Created"

// Report describes a trivy report along with its marshaler
type Report struct {
	id         string
	bom        *cyclonedx_v1_4.Bom
	osFallback bool
}

type reportOptions struct {
	dependencies    bool
	simplifyBomRefs bool
	osFallback      bool // the report lists the OS packages alone because the application scan ran out of time
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
	if opts.osFallback {
		appendSBOMOSFallback(bom)
	}

	bom14 := bomconvert.ConvertBOM(bom, opts.simplifyBomRefs)

	return &Report{
		id:         id,
		bom:        bom14,
		osFallback: opts.osFallback,
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

// appendSBOMOSFallback marks the SBOM as listing the OS packages alone.
func appendSBOMOSFallback(bom *cdx.BOM) {
	if bom.Metadata == nil {
		bom.Metadata = &cdx.Metadata{}
	}
	if bom.Metadata.Properties == nil {
		bom.Metadata.Properties = &[]cdx.Property{}
	}
	*bom.Metadata.Properties = append(*bom.Metadata.Properties, cdx.Property{
		Name:  sbom.OSFallbackProperty,
		Value: "true",
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

// OSFallback reports whether the report lists the OS packages alone because
// the scan with application analyzers ran out of time.
func (r *Report) OSFallback() bool {
	return r != nil && r.osFallback
}
