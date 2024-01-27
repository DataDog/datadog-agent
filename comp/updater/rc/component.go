// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package rc implements the remote config component.
package rc

import (
	"context"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/updater"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
	"go.uber.org/fx"
)

// team: fleet

// Module is the fx module for the updater rc client.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newRemoteConfigComponent),
	)
}

// Params contains the parameters to build the updater.
type Params struct {
	fx.In

	Ctx context.Context
	Log log.Component
}

func newRemoteConfigComponent(params Params) (*updater.RemoteConfig, error) {
	hostname, err := hostname.Get(params.Ctx)
	if err != nil {
		return nil, fmt.Errorf("could not get hostname: %w", err)
	}
	rc, err := updater.NewRemoteConfig(hostname)
	if err != nil {
		return nil, fmt.Errorf("could not create remote config: %w", err)
	}
	return rc, nil
}

// Component is the interface for the rc updater component.
type Component interface {
}
