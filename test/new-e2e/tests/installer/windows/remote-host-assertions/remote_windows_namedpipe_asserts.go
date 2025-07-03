// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package assertions

import (
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
)

// RemoteWindowsNamedPipeAssertions is a type that extends the RemoteWindowsHostAssertions to add assertions
// specific to a named pipe.
type RemoteWindowsNamedPipeAssertions struct {
	*RemoteWindowsHostAssertions
	pipename string
}

// WithSecurity asserts that the named pipe has the given security descriptor.
func (r *RemoteWindowsNamedPipeAssertions) WithSecurity(expected common.ObjectSecurity) *RemoteWindowsNamedPipeAssertions {
	r.context.T().Helper()
	actual, err := common.GetNamedPipeSecurityInfo(r.remoteHost, r.pipename)
	r.require.NoError(err)
	common.AssertEqualAccessSecurity(r.context.T(), r.pipename, expected, actual)
	if r.context.T().Failed() {
		r.context.T().FailNow()
	}
	return r
}
