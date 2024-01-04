// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package secrets

// Mock implements mock-specific methods for the resources component.
//
// Usage:
//
//	fxutil.Test[dependencies](
//	   t,
//	   resources.MockModule,
//	   fx.Replace(resources.MockParams{Data: someData}),
//	)
type Mock interface {
	Component

	// SetFetchHookFunc allows the caller to overwrite the function that resolves secrets (and exec the secret
	// binary).
	//
	// The mock function pass as parameter will receive a list of handle to resolve and should return a map with the
	// resolved value for each.
	//
	// Example:
	// a call like: fetchHookFunc([]{"a", "b", "c"})
	//
	// needs to return:
	//   map[string]string{
	//     "a": "resolved_value_for_a",
	//     "b": "resolved_value_for_b",
	//     "c": "resolved_value_for_c",
	//   }
	SetFetchHookFunc(func([]string) (map[string]string, error))

	// SetBackendCommand sets the backend command for resolving secrets
	SetBackendCommand(command string)
}
