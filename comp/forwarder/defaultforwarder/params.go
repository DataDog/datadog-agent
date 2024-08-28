// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package defaultforwarder

import (
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// Params contains the parameters to create a forwarder.
type Params struct {
	useNoopForwarder              bool
	withResolver                  bool
	disableAPIKeyCheckingOverride optional.Option[bool]
	features                      []Features
}

// NewParams initializes a new Params struct
func NewParams() Params {
	return Params{withResolver: false}
}

// NewParamsWithResolvers initializes a new Params struct with resolvers
func NewParamsWithResolvers() Params {
	return Params{withResolver: true}
}

// DisableAPIKeyChecking disables the API key checking
func (p *Params) DisableAPIKeyChecking() {
	p.disableAPIKeyCheckingOverride.Set(true)
}

// SetFeature adds a feature to the forwarder
func (p *Params) SetFeature(feature Features) {
	p.features = append(p.features, feature)
}

// UseNoopForwarder sets the forwarder to use the noop forwarder
func (p *Params) UseNoopForwarder() {
	p.useNoopForwarder = true
}
