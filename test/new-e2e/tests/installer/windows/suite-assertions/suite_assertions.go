// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package suiteasserts extends require.Assertions with custom assertions
package suiteasserts

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/components"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/installer/windows/remote-host-assertions"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// SuiteAssertions is a type to help write fluent, self-contained assertions.
// It is used as a bridge type to extend require.Assertions.
type SuiteAssertions struct {
	*require.Assertions
	suite suite.TestingSuite
}

// New creates a new SuiteAssertions
func New(r *require.Assertions, suite suite.TestingSuite) *SuiteAssertions {
	return &SuiteAssertions{
		Assertions: r,
		suite:      suite,
	}
}

// Host returns a RemoteWindowsHostAssertions to differentiates assertions running on the host vs assertions
// running remotely.
func (s *SuiteAssertions) Host(remoteHost *components.RemoteHost) *assertions.RemoteWindowsHostAssertions {
	return assertions.New(s.Assertions, s.suite, remoteHost)
}
