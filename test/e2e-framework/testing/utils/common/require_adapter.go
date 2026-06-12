// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package common

// RequireT wraps a Context to satisfy testify's require.TestingT interface,
// which requires a no-argument FailNow(). The adapter bridges the gap: Context
// exposes FailNow(format, args...) while testify expects FailNow().
//
// Usage:
//
//	require.NoError(common.RequireT{ctx}, err, "something failed")
type RequireT struct {
	Context
}

// FailNow satisfies require.TestingT by calling the underlying Context's
// FailNow with an empty message. Callers that want a message should pass it as
// a testify msgAndArgs argument, not via FailNow directly.
func (r RequireT) FailNow() { r.Context.FailNow("test assertion failed") }
