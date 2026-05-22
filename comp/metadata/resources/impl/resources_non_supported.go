// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || freebsd || netbsd || openbsd || solaris || dragonfly || aix

package resourcesimpl

import (
	compdef "github.com/DataDog/datadog-agent/comp/def"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	resources "github.com/DataDog/datadog-agent/comp/metadata/resources/def"
)

type resourcesImpl struct{}

// Requires defines the dependencies for the resources metadata component (non-supported platforms).
type Requires struct {
	compdef.In

	Log    log.Component
	Config config.Component
}

// Provides defines the output of the resources metadata component (non-supported platforms).
type Provides struct {
	compdef.Out

	Comp resources.Component
}

// NewComponent creates a new resources metadata component for non-supported platforms.
func NewComponent(_ Requires) Provides {
	return Provides{
		// We return a dummy Component
		Comp: &resourcesImpl{},
	}
}

// Get returns nil payload on unsuported platforms
func (r *resourcesImpl) Get() map[string]interface{} {
	return nil
}
