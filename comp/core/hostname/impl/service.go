// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package hostnameimpl implements the hostname component
package hostnameimpl

import (
	"context"

	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	pkghostname "github.com/DataDog/datadog-agent/pkg/util/hostname"
)

// Requires declares the input types to the hostname component constructor
type Requires struct {
	Lc compdef.Lifecycle
}

// Provides declares the output types from the hostname component constructor
type Provides struct {
	compdef.Out

	Comp hostname.Component
}

type service struct {
	ctx    context.Context
	cancel context.CancelFunc
}

var _ hostname.Component = (*service)(nil)

// NewComponent creates a new hostname component following the standard component pattern.
// It manages the hostname resolution lifecycle including drift detection.
func NewComponent(reqs Requires) (Provides, error) {
	ctx, cancel := context.WithCancel(context.Background())
	svc := &service{
		ctx:    ctx,
		cancel: cancel,
	}

	reqs.Lc.Append(compdef.Hook{
		OnStop: func(_ context.Context) error {
			// Cancel the context to stop drift detection goroutines
			cancel()
			return nil
		},
	})

	return Provides{Comp: svc}, nil
}

// NewHostnameService creates a hostname component without dependency injection.
// This is a convenience constructor for code that cannot use the fx framework.
// Prefer NewComponent for new code that uses dependency injection.
func NewHostnameService() hostname.Component {
	return &service{
		ctx:    context.Background(),
		cancel: func() {},
	}
}

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
func (hs *service) GetWithProvider(ctx context.Context) (hostname.Data, error) {
	return pkghostname.GetWithProvider(ctx)
}
