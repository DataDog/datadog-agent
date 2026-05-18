// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package fxutil

import "go.uber.org/fx"

// Supply provides a value of type T to the fx dependency graph as if it
// had been provided by a constructor that simply returns it.
//
// It is a typed wrapper around fx.Supply that avoids requiring a direct
// import of go.uber.org/fx in component modules that only need to supply
// a Params value.
func Supply[T any](value T) fx.Option {
	return fx.Supply(value)
}
