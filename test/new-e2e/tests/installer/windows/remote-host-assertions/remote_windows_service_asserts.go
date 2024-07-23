// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package assertions

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// RemoteWindowsServiceAssertions is a type that extends the RemoteWindowsHostAssertions to add assertions
// specific to a Windows service.
type RemoteWindowsServiceAssertions struct {
	*RemoteWindowsHostAssertions
	serviceConfig *common.ServiceConfig
}

// WithStatus asserts that the service has the given status.
func (r *RemoteWindowsServiceAssertions) WithStatus(expectedStatus string) *RemoteWindowsServiceAssertions {
	r.suite.T().Helper()
	actualStatus, err := common.GetServiceStatus(r.remoteHost, r.serviceConfig.ServiceName)
	r.require.NoError(err)
	r.require.Equal(expectedStatus, actualStatus)
	return r
}

// WithLogon asserts that the service runs under the given logon (username).
func (r *RemoteWindowsServiceAssertions) WithLogon(logon string) *RemoteWindowsServiceAssertions {
	r.suite.T().Helper()
	r.require.Equal(logon, r.serviceConfig.UserName)
	return r
}

// WithUserSid asserts that the service runs under the given SID.
func (r *RemoteWindowsServiceAssertions) WithUserSid(sid string) *RemoteWindowsServiceAssertions {
	r.suite.T().Helper()
	r.require.Equal(sid, r.serviceConfig.UserSID)
	return r
}
