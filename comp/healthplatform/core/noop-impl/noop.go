// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package noopimpl provides a no-op implementation of the health platform component
package noopimpl

import (
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/core/def"
	coreimpl "github.com/DataDog/datadog-agent/comp/healthplatform/core/impl"
)

// NewNoopComponent creates a no-op health platform component (disabled state)
func NewNoopComponent() healthplatform.Component {
	return coreimpl.NewNoopComponent()
}
