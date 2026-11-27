// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Imported from https://github.com/aquasecurity/trivy/blob/main/pkg/fanal/image/daemon/image.go

//go:build trivy

package trivy

import (
	"context"
	"fmt"
	"time"

	cdx "github.com/CycloneDX/cyclonedx-go"
	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	"github.com/DataDog/datadog-agent/pkg/sbom"
	"github.com/DataDog/datadog-agent/pkg/sbom/bomconvert"
	"github.com/DataDog/datadog-agent/pkg/sbom/telemetry"
	"github.com/DataDog/datadog-agent/pkg/sbom/usage"
	"github.com/aquasecurity/trivy/pkg/sbom/cyclonedx"
	sbomio "github.com/aquasecurity/trivy/pkg/sbom/io"
	"github.com/aquasecurity/trivy/pkg/types"
)

const imageCreatedPropertyKey = "datadog:image:Created"

// Report describes a trivy report along with its marshaler
type Report struct {
	id         string
	bom        *cyclonedx_v1_4.Bom
	osFallback bool
	usageIndex *usage.Index
}

type reportOptions struct {
	dependencies    bool
	simplifyBomRefs bool
	osFallback      bool // the report lists the OS packages alone because the application scan ran out of time

	// usageIndex builds the usage index of the SBOM, from a scan that started
	// at scanStart.
	usageIndex bool
	scanStart  time.Time
}

func newReport(id string, report *types.Report, marshaler cyclonedx.Marshaler, opts reportOptions) (*Report, error) {
	// The two steps of marshaler.MarshalReport, apart, so that the index
	// reads the bom-ref of each package between them.
	coreBOM, err := sbomio.NewEncoder(sbomio.WithBOMRef()).Encode(*report)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal report: %w", err)
	}
	var refs map[string][]string
	if opts.usageIndex {
		refs = componentRefs(coreBOM)
	}
	bom, err := marshaler.Marshal(context.TODO(), coreBOM)
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

	bom14, convertedRef := bomconvert.ConvertBOMWithRefs(bom, opts.simplifyBomRefs)

	var index *usage.Index
	if opts.usageIndex {
		index = buildUsageIndex(report, refs, convertedRef, bom14.GetSerialNumber(), opts.scanStart, time.Now())
		telemetry.SBOMUsageIndexSize.Observe(float64(index.Size()))
	}

	return &Report{
		id:         id,
		bom:        bom14,
		osFallback: opts.osFallback,
		usageIndex: index,
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

// UsageIndex returns the usage index of the SBOM of the report, nil when the
// collector builds none.
func (r *Report) UsageIndex() *usage.Index {
	if r == nil {
		return nil
	}
	return r.usageIndex
}
