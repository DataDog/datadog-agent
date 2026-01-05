// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.
// Copyright (c) 2021, RapDev.IO

// Package kubernetes provides wrappers for Kubernetes secret access
package kubernetes

import (
	"fmt"

	"github.com/DataDog/datadog-secret-backend/backend/file"
)

// NewK8sFileBackend returns a new Kubernetes file-based secrets backend
func NewK8sFileBackend(bc map[string]interface{}) (*file.TextFileBackend, error) {
	if _, exists := bc["secrets_path"]; !exists {
		return nil, fmt.Errorf("secrets_path is required for k8s.file backend")
	}

	return file.NewTextFileBackend(bc)
}
