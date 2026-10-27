// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package service

// MockPollOrgStatus performs one real status fetch without starting background
// polling. It lets consumers test the status contract with a loopback backend.
func MockPollOrgStatus(s *CoreAgentService) {
	s.orgStatusPoller.poll(s.api, s.rcType, s.status)
}
