// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package updaterimpl implements the updater component.
package updaterimpl

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	rcservice "github.com/DataDog/datadog-agent/comp/remote-config/rcservice/def"
	updatercomp "github.com/DataDog/datadog-agent/comp/updater/updater/def"
	"github.com/DataDog/datadog-agent/pkg/fleet/daemon"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var (
	errRemoteConfigRequired = errors.New("remote config is required to create the updater")
)

// Requires defines the dependencies for the updater component.
type Requires struct {
	Lifecycle compdef.Lifecycle

	Hostname     hostname.Component
	Log          log.Component
	Config       config.Component
	RemoteConfig option.Option[rcservice.Component]
}

// Provides defines the output of the updater component.
type Provides struct {
	Comp updatercomp.Component
}

// NewComponent creates a new updater component.
func NewComponent(reqs Requires) (Provides, error) {
	remoteConfig, ok := reqs.RemoteConfig.Get()
	if !ok {
		return Provides{}, errRemoteConfigRequired
	}
	hostname, err := reqs.Hostname.Get(context.Background())
	if err != nil {
		return Provides{}, fmt.Errorf("could not get hostname: %w", err)
	}
	d, err := daemon.NewDaemon(hostname, remoteConfig, reqs.Config)
	if err != nil {
		return Provides{}, fmt.Errorf("could not create updater: %w", err)
	}
	reqs.Lifecycle.Append(compdef.Hook{OnStart: d.Start, OnStop: d.Stop})
	return Provides{Comp: d}, nil
}
