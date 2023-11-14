// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build containerd && !trivy

package containerd

import (
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

// updateSBOMRepoMetadata does nothing
func updateSBOMRepoMetadata(sbom *workloadmeta.SBOM, _, _ []string) *workloadmeta.SBOM {
	return sbom
}
