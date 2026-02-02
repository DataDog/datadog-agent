// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package depvalidatorimpl implements the depvalidator component interface
package depvalidatorimpl

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	depvalidatordef "github.com/DataDog/datadog-agent/comp/logs-library/depvalidator/def"
)

const (
	// optionTypePrefix is the prefix of the option.Option type name (generics add type params like Option[string])
	optionTypePrefix = "Option["
	// optionPkgPath is the package path for option.Option
	optionPkgPath = "github.com/DataDog/datadog-agent/pkg/util/option"
	// structTagKey is the struct tag key used to mark fields as optional
	structTagKey = "depvalidator"
	// optionalTagValue marks a field as optional (won't be validated)
	optionalTagValue = "optional"
)

// depvalidatorImpl implements the depvalidator.Component interface
type depvalidatorImpl struct {
	log         log.Component
	logsEnabled bool
}

// Dependencies defines the dependencies for the depvalidator component
type Dependencies struct {
	Config config.Component
	Log    log.Component
}

// Provides contains the depvalidator component
type Provides struct {
	Comp depvalidatordef.Component
}

// NewProvides creates a new depvalidator component
func NewProvides(deps Dependencies) Provides {
	logsEnabled := deps.Config.GetBool("logs_enabled") || deps.Config.GetBool("log_enabled")

	if !logsEnabled {
		deps.Log.Debug("log collection is disabled, depsvalidator reliant components will not be instantiated")
	}
	return Provides{
		Comp: &depvalidatorImpl{
			log:         deps.Log,
			logsEnabled: logsEnabled,
		},
	}
}

// LogsEnabled returns true if the logs agent is enabled via configuration
func (o *depvalidatorImpl) LogsEnabled() bool {
	return o.logsEnabled
}

// ValidateDependencies checks that all option.Option[T] fields in the given
// struct have values set. Returns an error on the first field missing a value.
func (o *depvalidatorImpl) ValidateDependencies(deps any) error {
	v := reflect.ValueOf(deps)

	// Handle pointer to struct
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			err := errors.New("depvalidator: deps is nil")
			o.log.Error(err.Error())
			return err
		}
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		err := fmt.Errorf("depvalidator: deps must be a struct, got %s", v.Kind())
		o.log.Error(err.Error())
		return err
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		// Skip fields that aren't option.Option types
		if !isOptionType(field.Type) {
			continue
		}

		// Skip fields marked as optional via struct tag
		if tag, ok := field.Tag.Lookup(structTagKey); ok && tag == optionalTagValue {
			continue
		}

		// Check if the option has a value using the HasValue method
		hasValue, err := callHasValue(fieldValue)
		if err != nil {
			err = fmt.Errorf("depvalidator: error checking field %s: %w", field.Name, err)
			o.log.Error(err.Error())
			return err
		}

		if !hasValue {
			err := fmt.Errorf("depvalidator: required dependency %s is not set (logs are enabled but option.Option has no value)", field.Name)
			o.log.Error(err.Error())
			return err
		}
	}

	return nil
}

// ValidateIfEnabled combines LogsEnabled() and ValidateDependencies() for convenience.
// Returns nil if logs are enabled and deps are valid, ErrLogsDisabled if logs are disabled,
// or a validation error if deps are invalid.
func (o *depvalidatorImpl) ValidateIfEnabled(deps any) error {
	if !o.logsEnabled {
		return depvalidatordef.ErrLogsDisabled
	}
	return o.ValidateDependencies(deps)
}

// isOptionType checks if the given type is option.Option[T]
func isOptionType(t reflect.Type) bool {
	// Check if it's from the option package and has the Option prefix (generics include type params like Option[string])
	return t.PkgPath() == optionPkgPath && strings.HasPrefix(t.Name(), optionTypePrefix)
}

// callHasValue calls the HasValue method on an option.Option value
func callHasValue(v reflect.Value) (bool, error) {
	// Get the address of the value since HasValue has pointer receiver
	if !v.CanAddr() {
		// If we can't get address, create a copy we can address
		ptr := reflect.New(v.Type())
		ptr.Elem().Set(v)
		v = ptr.Elem()
	}

	method := v.Addr().MethodByName("HasValue")
	if !method.IsValid() {
		return false, errors.New("HasValue method not found")
	}

	results := method.Call(nil)
	if len(results) != 1 {
		return false, fmt.Errorf("HasValue returned unexpected number of values: %d", len(results))
	}

	return results[0].Bool(), nil
}
