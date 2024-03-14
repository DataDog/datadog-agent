// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmp

import (
	"fmt"
	"strings"
)

// OptPairs is just a useful type alias to avoid writing this out multiple times.
type OptPairs[T any] []struct {
	key string
	val T
}

// Options represents an ordered map of choices
type Options[T any] struct {
	Options map[string]T
	Order   []string
}

// OptsStr provides a '|'-delimited list of all nonempty options.
func (o Options[T]) OptsStr() string {
	var keys []string
	for _, k := range o.Order {
		if k == "" {
			continue
		}
		keys = append(keys, k)
	}
	return strings.Join(keys, "|")
}

// getOpt returns the key that matches choice, case-insensitively.
func (o Options[T]) getOpt(choice string) (string, bool) {
	choice = strings.ToLower(choice)
	for opt := range o.Options {
		if choice == strings.ToLower(opt) {
			return opt, true
		}
	}
	return "", false
}

// getVal returns the value whose key matches choice, case-insensitively.
func (o Options[T]) getVal(choice string) (T, bool) {
	if key, ok := o.getOpt(choice); ok {
		return o.Options[key], true
	}
	// return a zero-value T
	var t T
	return t, false
}

// Flag creates a flag using these options and storing the selected choice in target.
func (o Options[T]) Flag(target *string) OptionsFlag[T] {
	return OptionsFlag[T]{o, target, "option"}
}

// TypedFlag is the same as Flag but lets you customize how the type of flag is shown in the help.
func (o Options[T]) TypedFlag(target *string, typeName string) OptionsFlag[T] {
	return OptionsFlag[T]{o, target, typeName}
}

// NewOptions creates a new Options object from a set of pairs.
// We don't just create one directly from a map because map iteration order is random.
func NewOptions[T any](pairs OptPairs[T]) Options[T] {
	var order []string
	opts := make(map[string]T)
	for _, pair := range pairs {
		order = append(order, pair.key)
		opts[pair.key] = pair.val
	}
	return Options[T]{opts, order}
}

// OptionsFlag is an implementation of pflag.Value that complains when set to an invalid option.
type OptionsFlag[T any] struct {
	opts     Options[T]
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
	if opt, ok := a.opts.getOpt(p); ok {
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
