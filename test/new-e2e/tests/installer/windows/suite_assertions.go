// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/environments"
	"github.com/stretchr/testify/require"
	"testing"
)

// SuiteAssertions is a type to help write fluent, self-contained assertions.
// It extends require.Assertions
type SuiteAssertions struct {
	*require.Assertions
	testing *testing.T
	// The environment that we run with.
	// This should really be generic but for now it's limited to a Windows host.
	env *environments.WindowsHost
}

// Host returns a RemoteHostAssertions.
func (s *SuiteAssertions) Host() *RemoteHostAssertions {
	return &RemoteHostAssertions{
		SuiteAssertions: s,
		remoteHost:      s.env.RemoteHost,
	}
}
