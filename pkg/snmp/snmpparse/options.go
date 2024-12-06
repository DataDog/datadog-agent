// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package snmpparse

import "strings"

// OptPairs is just a useful type alias to avoid writing this out multiple times.
type OptPairs[T any] []struct {
	Key string
	Val T
}

// Options represents an ordered map of case-insensitive choices.
// This is particularly useful for generating sensible error messages
// and command-line documentation.
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

// GetOpt returns the key that matches choice, case-insensitively.
// The key is found case-insensitively; the returned value will be
// the key as defined in the original list.
func (o Options[T]) GetOpt(choice string) (string, bool) {
	choice = strings.ToLower(choice)
	for opt := range o.Options {
		if choice == strings.ToLower(opt) {
			return opt, true
		}
	}
	return "", false
}

// GetVal returns the value whose key matches choice, case-insensitively.
func (o Options[T]) GetVal(choice string) (T, bool) {
	if key, ok := o.GetOpt(choice); ok {
		return o.Options[key], true
	}
	// return a zero-value T
	var t T
	return t, false
}

// NewOptions creates a new Options object from a slice of pairs.
// We don't just create one directly from a map because map iteration order is random.
func NewOptions[T any](pairs OptPairs[T]) Options[T] {
	var order []string
	opts := make(map[string]T)
	for _, pair := range pairs {
		order = append(order, pair.Key)
		opts[pair.Key] = pair.Val
	}
	return Options[T]{opts, order}
}
