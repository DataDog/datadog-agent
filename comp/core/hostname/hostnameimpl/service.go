// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package hostnameimpl

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	pkghostname "github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/fx"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newHostnameService))
}

type service struct{}

var _ hostname.Component = (*service)(nil)

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

// TODO: (components) remove once compliance, epforwarder, and security are FX components
//
//nolint:revive // TODO(AML) Fix revive linter
func NewHostnameService() hostname.Component {
	return newHostnameServiceCompat()
}

// TODO: (components) - merge with newHostnameServiceCompat once NewHostnameService is removed
func newHostnameService() hostname.Component {
	return newHostnameServiceCompat()
}

func newHostnameServiceCompat() hostname.Component {
	return &service{}
}
