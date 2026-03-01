// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package localapiclientimpl provides the local API client component.
package localapiclientimpl

import (
	localapiclient "github.com/DataDog/datadog-agent/comp/updater/localapiclient/def"
	"github.com/DataDog/datadog-agent/pkg/fleet/daemon"
)

// Requires defines the dependencies for the local API client component.
type Requires struct{}

// Provides defines the output of the local API client component.
type Provides struct {
	Comp localapiclient.Component
}

// NewComponent creates a new local API client component.
func NewComponent(_ Requires) (Provides, error) {
	return Provides{Comp: daemon.NewLocalAPIClient()}, nil
}
