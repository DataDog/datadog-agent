// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package rdnsquerierimpl implements the reverse DNS querier component.
package rdnsquerierimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/rdnsquerier"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In
	Lc fx.Lifecycle
}

type provides struct {
	fx.Out
	Comp rdnsquerier.Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRDNSQuerier),
	)
}

type rdnsQuerierImpl struct {
	lc fx.Lifecycle
}

func newRDNSQuerier(deps dependencies) provides {
	// Component initialization
	querier := &rdnsQuerierImpl{
		lc: deps.Lc,
	}
	return provides{
		Comp: querier,
	}
}

// GetHostname gets the hostname for the given IP address if the IP address is in the private address space, or returns an empty string if not.
// The initial implementation always returns an empty string.
func (q *rdnsQuerierImpl) GetHostname(_ []byte) string {
	return ""
}
