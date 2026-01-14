// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package impl implements the validator component interface
package impl

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/validator/def"
)

// Dependencies contains the required dependencies for the validator component.
type Dependencies struct {
	Config config.Component
	Log    log.Component
}

// Provides contains the validator component.
type Provides struct {
	Validator def.Component
}

// Validator checks that logs are enabled and that required optional dependencies are present.
type Validator struct {
	config config.Component
	log    log.Component
}

// NewProvides creates a new validator component instance.
// The validator is used by other logs-library components to reduce boilerplate
// when checking if logs are enabled and if required dependencies are present.
func NewProvides(deps Dependencies) Provides {
	return Provides{
		Validator: &Validator{
			config: deps.Config,
			log:    deps.Log,
		},
	}
}

// ValidateDependencies validates that logs are enabled and all provided optional dependencies have values.
func (v *Validator) ValidateDependencies(options ...def.Option) error {
	var msg string
	if !v.config.GetBool("logs_enabled") || !v.config.GetBool("log_enabled") {
		msg = "logs are disabled - check 'logs_enabled' and 'log_enabled' config keys"
		v.log.Error(msg)
		return errors.New(msg)
	}
	for i, option := range options {
		if !option.HasValue() {
			// Use index since we can't reliably get the actual field name from the option
			msg = fmt.Sprintf("Required dependency %d is not set (type: %s)", i, reflect.TypeOf(option))
			v.log.Error(msg)
			return errors.New(msg)
		}
	}

	return nil
}
