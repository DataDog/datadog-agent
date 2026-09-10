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
	sbomio "github.com/aquasecurity/trivy/pkg/sbom/io"
	"github.com/aquasecurity/trivy/pkg/types"
)

// Report describes a trivy report along with its marshaler
type Report struct {
	id            string
	bom           *cyclonedx_v1_4.Bom
	indexID       string
	componentRefs map[string]string
}

type reportOptions struct {
	dependencies    bool
	simplifyBomRefs bool
}

func newReport(id string, report *types.Report, marshaler cyclonedx.Marshaler, opts reportOptions) (*Report, error) {
	// Keep the intermediate BOM so the usage index can follow the exact refs
	// Trivy chose for duplicate-PURL and PURL-less component occurrences. Calling
	// MarshalReport would hide this identity assignment behind the final output.
	coreBOM, err := sbomio.NewEncoder(sbomio.WithBOMRef()).Encode(*report)
	if err != nil {
		return nil, err
	}

	// Components finalizes BOM refs lazily. UID is used only as an in-process
	// bridge back to the packages in the source report; ambiguous UIDs are left
	// out rather than silently attaching usage to the wrong occurrence.
	rawRefsByUID := make(map[string][]string)
	rawRefCounts := make(map[string]int)
	for _, component := range coreBOM.Components() {
		uid := component.PkgIdentifier.UID
		bomRef := component.PkgIdentifier.BOMRef
		if bomRef == "" {
			continue
		}
		rawRefCounts[bomRef]++
		if uid != "" {
			rawRefsByUID[uid] = append(rawRefsByUID[uid], bomRef)
		}
	}

	bom, err := marshaler.Marshal(context.TODO(), coreBOM)
	if err != nil {
		return nil, err
	}

	if !opts.dependencies {
		bom.Dependencies = nil
	}

	bom14, convertedRefs := bomconvert.ConvertBOMWithBOMRefMapping(bom, opts.simplifyBomRefs)
	componentRefs := make(map[string]string, len(rawRefsByUID))
	for uid, rawRefs := range rawRefsByUID {
		if len(rawRefs) != 1 || rawRefCounts[rawRefs[0]] != 1 {
			continue
		}
		if converted := convertedRefs[rawRefs[0]]; converted != "" {
			componentRefs[uid] = converted
		}
	}

	return &Report{
		id:            id,
		bom:           bom14,
		indexID:       bom14.GetSerialNumber(),
		componentRefs: componentRefs,
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
