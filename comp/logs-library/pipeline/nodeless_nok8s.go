// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !kubelet

package pipeline

import (
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// isNodelessNode always returns false when not built with kubelet support.
func isNodelessNode(_ pkgconfigmodel.Reader) bool {
	return false
}
