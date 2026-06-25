// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fxinstrumentationimpl

import fxinstrumentation "github.com/DataDog/datadog-agent/comp/core/fxinstrumentation/def"

// Provides defines the output of the fxinstrumentation component.
type Provides struct {
	Comp fxinstrumentation.Component
}

// NewComponent creates a new fxinstrumentation component.
func NewComponent() Provides {
	return Provides{Comp: struct{}{}}
}
