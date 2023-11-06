// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !trivy

package workloadmeta

// updateSBOMRepoMetadata does nothing
func updateSBOMRepoMetadata(sbom *SBOM, _, _ []string) *SBOM {
	return sbom
}
