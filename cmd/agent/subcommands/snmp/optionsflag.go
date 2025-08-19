// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmp

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
)

// Flag creates a flag using these options and storing the selected choice in target.
func Flag[T any](options *snmpparse.Options[T], target *string) OptionsFlag[T] {
	return OptionsFlag[T]{options, target, "option"}
}

// TypedFlag is the same as Flag but lets you customize how the type of flag is shown in the help.
func TypedFlag[T any](options *snmpparse.Options[T], target *string, typeName string) OptionsFlag[T] {
	return OptionsFlag[T]{options, target, typeName}
}

// OptionsFlag is an implementation of pflag.Value that complains when set to an invalid option.
type OptionsFlag[T any] struct {
	opts     *snmpparse.Options[T]
	value    *string
	typeName string
}

// String returns a string representation of this value.
func (a OptionsFlag[T]) String() string {
	if a.value == nil {
		return ""
	}
	return *a.value
}

// Set sets the value, returning an error if the given choice isn't valid.
func (a OptionsFlag[T]) Set(p string) error {
	if opt, ok := a.opts.GetOpt(p); ok {
		*a.value = opt
		return nil
	}
	// note: we don't need to put p itself in this error because cobra prints the flag and the value as part of the error. For example:
	// Error: invalid argument "foo" for "-x, --priv-protocol" flag: must be one of AES|AES192|AES192C|AES256|AES256C|DES
	return fmt.Errorf("must be one of %s", a.opts.OptsStr())
}

// Type is how the value is represented in the auto-generated help.
func (a OptionsFlag[T]) Type() string {
	return a.typeName
}
