// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows && !darwin

// Package logondurationimpl implements the logon duration component
package logondurationimpl

import (
	configcomp "github.com/DataDog/datadog-agent/comp/core/config"
	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	logcomp "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	logonduration "github.com/DataDog/datadog-agent/comp/logonduration/def"
)

// Requires defines the dependencies for the logon duration component
type Requires struct {
	Lc             compdef.Lifecycle
	Config         configcomp.Component
	SysprobeConfig sysprobeconfig.Component
	Log            logcomp.Component
	EventPlatform  eventplatform.Component
	Hostname       hostname.Component
}

// Provides defines what this component provides
type Provides struct {
	Comp logonduration.Component
}

type logonDurationComponent struct{}

// NewComponent creates a noop logon duration component for unsupported platforms
func NewComponent(_ Requires) Provides {
	return Provides{
		Comp: &logonDurationComponent{},
	}
}
