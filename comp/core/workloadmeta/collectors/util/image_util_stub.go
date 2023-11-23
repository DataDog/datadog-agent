// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build !trivy

// Package util contains utility functions for image metadata collection
package util

import (
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
)

// UpdateSBOMRepoMetadata does nothing
func UpdateSBOMRepoMetadata(sbom *workloadmeta.SBOM, _, _ []string) *workloadmeta.SBOM {
	return sbom
}
