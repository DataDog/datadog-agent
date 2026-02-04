// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package sbomutil

import (
	"bytes"
	"compress/gzip"
	"io"

	"github.com/DataDog/agent-payload/v5/cyclonedx_v1_4"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"google.golang.org/protobuf/proto"
)

// CompressSBOM converts a workloadmeta.SBOM into a workloadmeta.CompressedSBOM.
func CompressSBOM(sbom *workloadmeta.SBOM) (*workloadmeta.CompressedSBOM, error) {
	if sbom == nil {
		return nil, nil
	}

	uncompressedBom, err := proto.Marshal(sbom.CycloneDXBOM)
	if err != nil {
		return nil, err
	}

	var compressedBom bytes.Buffer
	writer := gzip.NewWriter(&compressedBom)
	if _, err := writer.Write(uncompressedBom); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	return &workloadmeta.CompressedSBOM{
		Bom:                compressedBom.Bytes(),
		GenerationTime:     sbom.GenerationTime,
		GenerationDuration: sbom.GenerationDuration,
		GenerationMethod:   sbom.GenerationMethod,
		Status:             sbom.Status,
		Error:              sbom.Error,
	}, nil
}

// UncompressSBOM converts a workloadmeta.CompressedSBOM into a workloadmeta.SBOM.
func UncompressSBOM(csbom *workloadmeta.CompressedSBOM) (*workloadmeta.SBOM, error) {
	if csbom == nil {
		return nil, nil
	}

	reader, err := gzip.NewReader(bytes.NewReader(csbom.Bom))
	if err != nil {
		return nil, err
	}

	uncompressedBom, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if err := reader.Close(); err != nil {
		return nil, err
	}

	var cyclosbom cyclonedx_v1_4.Bom
	if err := proto.Unmarshal(uncompressedBom, &cyclosbom); err != nil {
		return nil, err
	}

	return &workloadmeta.SBOM{
		CycloneDXBOM:       &cyclosbom,
		GenerationTime:     csbom.GenerationTime,
		GenerationDuration: csbom.GenerationDuration,
		GenerationMethod:   csbom.GenerationMethod,
		Status:             csbom.Status,
		Error:              csbom.Error,
	}, nil
}
