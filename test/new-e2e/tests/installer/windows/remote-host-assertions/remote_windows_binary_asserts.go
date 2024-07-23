// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package assertions provide custom assertions for Windows tests
package assertions

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/stretchr/testify/assert"
	"strings"
)

// RemoteWindowsBinaryAssertions is a type that extends the RemoteWindowsHostAssertions to add assertions
// specific to a Windows binary.
type RemoteWindowsBinaryAssertions struct {
	*RemoteWindowsHostAssertions
	binaryPath string
}

// WithSignature verifies the authenticode signature of the binary. This test does not call `FailNow` in case
// the signature does not match.
func (r *RemoteWindowsBinaryAssertions) WithSignature(expectedSignatures map[string]struct{}) *RemoteWindowsBinaryAssertions {
	r.suite.T().Helper()
	verify, _ := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.VerifyCodeSignature, true)

	if verify {
		sig, err := common.GetAuthenticodeSignature(r.remoteHost, r.binaryPath)
		// Use assert instead of require since we don't want to stop the test right here but continue executing
		// other assertions (i.e. this failure is non-terminal).
		assert.NoError(r.suite.T(), err, "could not get authenticode signature for binary")
		assert.True(r.suite.T(), sig.Valid(), "binary signature was not valid")
		if _, ok := expectedSignatures[strings.ToUpper(sig.SignerCertificate.Thumbprint)]; !ok {
			assert.Error(r.suite.T(), fmt.Errorf("signature thumbprint is not valid: %s", sig.SignerCertificate.Thumbprint))
		}
	}

	return r
}

// WithVersionEqual verifies the version of a binary matches what's expected by calling "version" on it.
// Obviously the binary must support the "version" command, which is normally the case for most Agent binaries.
func (r *RemoteWindowsBinaryAssertions) WithVersionEqual(expected string) *RemoteWindowsBinaryAssertions {
	r.suite.T().Helper()
	return r.WithVersionMatchPredicate(func(actual string) {
		r.require.Equal(expected, actual, "version should be equal")
	})
}

// WithVersionNotEqual verifies the version of a binary NOT match the expected by calling "version" on it.
// Obviously the binary must support the "version" command, which is normally the case for most Agent binaries.
func (r *RemoteWindowsBinaryAssertions) WithVersionNotEqual(expected string) *RemoteWindowsBinaryAssertions {
	r.suite.T().Helper()
	return r.WithVersionMatchPredicate(func(actual string) {
		r.require.NotEqual(expected, actual, "version should not be equal")
	})
}

// WithVersionMatchPredicate obtains the binary version by calling "version" and uses the predicate to verify
// if the version match the expectations.
func (r *RemoteWindowsBinaryAssertions) WithVersionMatchPredicate(predicate func(version string)) *RemoteWindowsBinaryAssertions {
	r.suite.T().Helper()
	actual, err := r.remoteHost.Execute(fmt.Sprintf("& \"%s\" %s", r.binaryPath, "version"))
	r.require.NoError(err)
	actualVersion, err := version.New(strings.TrimSuffix(actual, "\n"), "")
	r.require.NoErrorf(err, "invalid actual version %s", actual)
	predicate(actualVersion.GetNumberAndPre())
	return r
}
