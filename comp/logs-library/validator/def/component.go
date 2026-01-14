// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package def implements the validator component interface
package def

import "github.com/DataDog/datadog-agent/pkg/util/option"

// Option represents any type that can be validated for presence.
// This interface provides type erasure, allowing option.Option[T] of different types
// to be passed together in the ValidateDependencies variadic parameter.
//
// The pkg/util/option.Option[T] type implements this interface via its HasValue() method.
type Option interface {
	HasValue() bool
}

// Component is the validator component interface.
// It provides centralized validation logic for logs-library components,
// reducing boilerplate code for checking if logs are enabled and if required
// optional dependencies are present.
type Component interface {
	// ValidateDependencies validates that:
	// 1. Both 'logs_enabled' and 'log_enabled' config keys are true
	// 2. All provided options have values (HasValue() returns true)
	//
	// Returns an error if any validation fails, nil if all checks pass.
	// Errors are logged automatically by the validator.
	ValidateDependencies(options ...Option) error
}

// GenOption generates an option.Option[P] for a component by validating dependencies first.
// This serves as boilerplate reduction for the logs agent, which must have virtually all of
// its fx dependencies set to None if the agent is disabled.
//
// The function validates that logs are enabled and that all requiredOptions have values.
// If validation passes, it calls the constructor with deps and returns option.New(result).
// If validation fails, it returns option.None[P]().
//
// Type Parameters:
//   - P: The Provides/Requires type
//   - D: The Dependencies type
//
// Parameters:
//   - component: The validator component instance
//   - deps: The dependencies struct (passed to constructor if validation passes)
//   - constructor: Function that constructs the component from dependencies
//   - requiredOptions: Optional dependencies that must have values for construction to proceed
//
// Usage Examples:
//
//	type Dependencies struct {
//	    Validator validatordef.Component
//	    Log       option.Option[log.Component] // must be present
//	    Config    option.Option[config.Component] // can be None
//	}
//
//	func NewProvides(deps Dependencies) Provides {
//	    return Provides{
//	        Comp: validatordef.GenOption(
//	            deps.Validator,
//	            deps,
//	            newComponent,
//	            &deps.Log,
//	        ),
//	    }
//	}
func GenOption[P any, D any](component Component, deps D, constructor func(D) P, requiredOptions ...Option) option.Option[P] {
	if err := component.ValidateDependencies(requiredOptions...); err != nil {
		return option.None[P]()
	}
	return option.New(constructor(deps))
}
