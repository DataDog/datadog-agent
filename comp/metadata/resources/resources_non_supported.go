// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows || freebsd || netbsd || openbsd || solaris || dragonfly

package resources

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"go.uber.org/fx"
)

type resources struct{}

type provides struct {
	fx.Out

	Comp Component
}

func newResourcesProvider(log log.Component, config config.Component) provides {
	return provides{
		// We return a dummy Component
		Comp: &resources{},
	}
}

// Get returns nil payload on unsuported platforms
func (r *resources) Get() map[string]interface{} {
	return nil
}
