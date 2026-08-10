// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package config

// WithPendingDelegatedAuthForTest returns a copy of the endpoint marked (or unmarked) as
// delegated-auth-managed, for exercising the WIF-aware 403 retry path in tests.
func (e Endpoint) WithPendingDelegatedAuthForTest(pending bool) Endpoint {
	e.hasPendingDelegatedAuth = pending
	return e
}
