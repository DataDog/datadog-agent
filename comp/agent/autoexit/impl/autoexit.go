// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package autoexitimpl implements autoexit.Component
package autoexitimpl

import (
	autoexit "github.com/DataDog/datadog-agent/comp/agent/autoexit/def"
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	pkgcommon "github.com/DataDog/datadog-agent/pkg/util/common"
)

// Requires defines the dependencies for the autoexit component.
type Requires struct {
	Config config.Component
	Log    log.Component
}

// Provides defines the output of the autoexit component.
type Provides struct {
	Comp autoexit.Component
}

// NewComponent creates a new autoexit component.
func NewComponent(reqs Requires) (Provides, error) {
	ctx, _ := pkgcommon.GetMainCtxCancel()
	err := configureAutoExit(ctx, reqs.Config, reqs.Log)
	return Provides{Comp: struct{}{}}, err
}
