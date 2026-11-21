// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package apmattachimpl implements the apmattach component.
package apmattachimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/status"
	compdef "github.com/DataDog/datadog-agent/comp/def"

	apmattach "github.com/DataDog/datadog-agent/comp/apmattach/def"
)

type Requires struct {
	Lifecycle compdef.Lifecycle
	Log       log.Component
	Config    config.Component
}

type Provides struct {
	Comp   apmattach.Component
	Status status.InformationProvider
}

func NewComponent(reqs Requires) (Provides, error) {
	return newComponent(reqs)
}
