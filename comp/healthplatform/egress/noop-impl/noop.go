// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package noopimpl provides a no-op implementation of the health platform egress component.
// Used when the health platform is disabled via configuration.
package noopimpl

import (
	egressdef "github.com/DataDog/datadog-agent/comp/healthplatform/egress/def"
)

type noopEgress struct{}

// NewNoopComponent creates a no-op egress (health platform disabled).
func NewNoopComponent() egressdef.Component {
	return &noopEgress{}
}
