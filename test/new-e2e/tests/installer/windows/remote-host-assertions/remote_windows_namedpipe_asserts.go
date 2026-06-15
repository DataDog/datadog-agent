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
	contextT(r.context).Helper()
	actual, err := common.GetNamedPipeSecurityInfo(r.remoteHost, r.pipename)
	r.require.NoError(err)
	common.AssertEqualAccessSecurity(contextT(r.context), r.pipename, expected, actual)
	if contextT(r.context).Failed() {
		contextT(r.context).FailNow()
	}
	return r
}
