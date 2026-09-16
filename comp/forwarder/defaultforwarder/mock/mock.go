// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package defaultforwardermock provides a mock forwarder component for testing.
package defaultforwardermock

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	secrets "github.com/DataDog/datadog-agent/comp/core/secrets/def"
	defaultforwarderdef "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	defaultforwarderimpl "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Mock implements mock-specific methods.
type Mock interface {
	defaultforwarderdef.Component
}

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMockComponent),
	)
}

func newMockComponent(config config.Component, log log.Component, secrets secrets.Component) defaultforwarderdef.Component {
	options, _ := defaultforwarderimpl.NewOptions(config, log, nil)
	if options == nil {
		options = &defaultforwarderimpl.Options{}
	}
	options.Secrets = secrets
	return defaultforwarderimpl.NewDefaultForwarder(config, log, options)
}
