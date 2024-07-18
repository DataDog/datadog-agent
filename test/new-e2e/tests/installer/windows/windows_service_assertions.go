// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// WindowsServiceAssertions represents fluent assertions for a Windows service
type WindowsServiceAssertions struct {
	*SuiteAssertions
	serviceConfig *common.ServiceConfig
}

// WithStatus asserts that the service has the given status.
func (require *WindowsServiceAssertions) WithStatus(status string) *WindowsServiceAssertions {
	require.testing.Helper()
	status, err := common.GetServiceStatus(require.env.RemoteHost, require.serviceConfig.ServiceName)
	require.NoError(err)
	require.Equal(status, status)
	return require
}

// WithLogon asserts that the service runs under the given logon (username).
func (require *WindowsServiceAssertions) WithLogon(logon string) *WindowsServiceAssertions {
	require.testing.Helper()
	require.Equal(logon, require.serviceConfig.UserName)
	return require
}

// WithUserSid asserts that the service runs under the given SID.
func (require *WindowsServiceAssertions) WithUserSid(sid string) *WindowsServiceAssertions {
	require.testing.Helper()
	require.Equal(sid, require.serviceConfig.UserSID)
	return require
}
