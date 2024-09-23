// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package assertions

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// RemoteWindowsRegistryKeyAssertions is a type that extends the RemoteWindowsHostAssertions to add assertions
// specific to a Windows registry.
type RemoteWindowsRegistryKeyAssertions struct {
	*RemoteWindowsHostAssertions
	keyPath string
}

// WithValueEqual verifies the value of a registry key matches what's expected.
func (r *RemoteWindowsRegistryKeyAssertions) WithValueEqual(value, expected string) *RemoteWindowsRegistryKeyAssertions {
	r.suite.T().Helper()
	actual, err := common.GetRegistryValue(r.remoteHost, r.keyPath, value)
	r.require.NoError(err, "could not get registry value")
	r.require.Equal(expected, actual, "registry value should be equal")
	return r
}
