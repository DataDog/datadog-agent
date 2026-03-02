// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !windows

package logondurationimpl

import (
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Requires defines the dependencies for the logon duration component on unsupported platforms.
type Requires struct {
	Lc compdef.Lifecycle
}

type logonDurationComponent struct{}

// NewComponent creates a no-op logon duration component on unsupported platforms.
func NewComponent(reqs Requires) Provides {
	log.Debug("Logon duration component is not supported on this platform")
	return Provides{
		Comp: &logonDurationComponent{},
	}
}
