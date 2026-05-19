// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package configsenderimpl

import (
	configsender "github.com/DataDog/datadog-agent/comp/discovery/configsender/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
)

// Requires defines the dependencies for the configsender component.
// On non-linux platforms the component is a no-op; the discovery rust
// module only runs on linux.
type Requires struct {
	Config   pkgconfigmodel.Reader
	Hostname hostnameinterface.Component
}

// Provides defines the output of the configsender component.
type Provides struct {
	Comp configsender.Component
}

type noopSender struct{}

// NewComponent returns a no-op component on non-linux builds.
func NewComponent(_ Requires) Provides {
	log.Info("configsender: noop on non-linux platforms")
	return Provides{Comp: &noopSender{}}
}
