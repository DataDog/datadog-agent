// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package service

// MockOrgStatus binds an INJECTED observation for consumer tests, not production.
func MockOrgStatus(key, endpoint string, value OrgStatus) OrgStatus {
	value.binding = orgStatusBinding(key, endpoint)
	return value
}

// MockPARScope binds an INJECTED grant; it is not evidence of an AAA request.
func MockPARScope(key, endpoint string, value PARScopeObservation) PARScopeObservation {
	value.binding = orgStatusBinding(key, endpoint)
	return value
}
