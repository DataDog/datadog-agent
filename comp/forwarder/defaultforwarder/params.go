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
	UseNoopForwarder              bool
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

func (p *Params) DisableAPIKeyChecking() {
	p.disableAPIKeyCheckingOverride.Set(true)
}

func (p *Params) SetFeature(feature Features) {
	p.features = append(p.features, feature)
}
