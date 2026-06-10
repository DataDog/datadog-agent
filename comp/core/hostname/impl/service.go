// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package hostnameimpl implements the component hostname
package hostnameimpl

import (
	"context"

	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/def"
	pkghostname "github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// Requires defines the dependencies for the hostname component
type Requires struct{}

// Provides defines the output of the hostname component
type Provides struct {
	Comp hostname.Component
}

type service struct{}

// Get returns the hostname.
func (hs *service) Get(ctx context.Context) (string, error) {
	return pkghostname.Get(ctx)
}

// GetSafe returns the hostname, or 'unknown host' if anything goes wrong.
func (hs *service) GetSafe(ctx context.Context) string {
	name, err := hs.Get(ctx)
	if err != nil {
		return "unknown host"
	}
	return name
}

// GetWithProvider returns the hostname for the Agent and the provider that was use to retrieve it.
func (hs *service) GetWithProvider(ctx context.Context) (pkghostname.Data, error) {
	return pkghostname.GetWithProvider(ctx)
}

// NewComponent creates a new instance of the component hostname
func NewComponent(Requires) Provides {
	return Provides{Comp: &service{}}
}
