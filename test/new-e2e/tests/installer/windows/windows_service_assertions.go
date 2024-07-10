// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/stretchr/testify/assert"
)

// WindowsServiceAssertions represents fluent assertions for a Windows service
type WindowsServiceAssertions struct {
	*SuiteAssertions
	serviceConfig *common.ServiceConfig
}

// WithStatus asserts that the service has the given status.
func (serviceAssertions *WindowsServiceAssertions) WithStatus(status string) *WindowsServiceAssertions {
	status, err := common.GetServiceStatus(serviceAssertions.env.RemoteHost, serviceAssertions.serviceConfig.ServiceName)
	assert.NoError(serviceAssertions.testing, err)
	assert.Equal(serviceAssertions.testing, status, status)
	return serviceAssertions
}

// WithLogon asserts that the service runs under the given logon (username).
func (serviceAssertions *WindowsServiceAssertions) WithLogon(logon string) *WindowsServiceAssertions {
	assert.Equal(serviceAssertions.testing, logon, serviceAssertions.serviceConfig.UserName)
	return serviceAssertions
}

// WithUserSid asserts that the service runs under the given SID.
func (serviceAssertions *WindowsServiceAssertions) WithUserSid(sid string) *WindowsServiceAssertions {
	assert.Equal(serviceAssertions.testing, sid, serviceAssertions.serviceConfig.UserSID)
	return serviceAssertions
}
