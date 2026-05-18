// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package fxutil

import (
	"reflect"

	"go.uber.org/fx"
)

// Supply provides a value of type T to the fx dependency graph as if it
// had been provided by a constructor that simply returns it.
//
// It is a typed wrapper around fx.Supply that:
//   - Avoids requiring a direct import of go.uber.org/fx in component modules.
//   - Fixes a footgun in fx.Supply: when T is an interface type, fx.Supply
//     uses the concrete type of the value (not the declared interface), which
//     is almost never the intent. Supply detects this and uses fx.Annotate to
//     preserve the declared interface type instead.
func Supply[T any](value T) fx.Option {
	declaredType := reflect.TypeOf((*T)(nil)).Elem()
	if declaredType.Kind() == reflect.Interface {
		// fx.Supply(value) would provide the concrete type of value, not T.
		// Use fx.Annotate so the graph sees T (the declared interface type).
		return fx.Supply(fx.Annotate(value, fx.As(new(T))))
	}
	return fx.Supply(value)
}
