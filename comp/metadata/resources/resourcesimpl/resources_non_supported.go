// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || freebsd || netbsd || openbsd || solaris || dragonfly

package resourcesimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/metadata/resources"
	"go.uber.org/fx"
)

type resourcesImpl struct{}

type provides struct {
	fx.Out

	Comp resources.Component
}

func newResourcesProvider(_ log.Component, _ config.Component) provides {
	return provides{
		// We return a dummy Component
		Comp: &resourcesImpl{},
	}
}

// Get returns nil payload on unsuported platforms
func (r *resourcesImpl) Get() map[string]interface{} {
	return nil
}
