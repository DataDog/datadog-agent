// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package installerwindows

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/version"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-agent/test/new-e2e/tests/windows/common"
	"github.com/stretchr/testify/assert"
	"strings"
)

type remoteBinaryAssertions struct {
	*RemoteHostAssertions
	binaryPath string
}

// WithSignature verifies the authenticode signature of the binary. This test does not call `FailNow` in case
// the signature does not match.
func (r *remoteBinaryAssertions) WithSignature(expectedSignature string) *remoteBinaryAssertions {
	r.testing.Helper()
	verify, _ := runner.GetProfile().ParamStore().GetBoolWithDefault(parameters.VerifyCodeSignature, true)

	if verify {
		sig, err := common.GetAuthenticodeSignature(r.remoteHost, r.binaryPath)
		// Use assert instead of require since we don't want to stop the test right here but continue executing
		// other assertions (i.e. this failure is non-terminal).
		assert.NoError(r.testing, err, "could not get authenticode signature for binary")
		assert.True(r.testing, sig.Valid(), "binary signature was not valid")
		assert.True(r.testing, strings.EqualFold(sig.SignerCertificate.Thumbprint, expectedSignature), "the binary signature did not match the expected signature")
	}

	return r
}

// WithVersion verifies the version of a binary by calling "version" on it.
// Obviously the binary must support the "version" command, which is normally the case for most Agent binaries.
func (r *remoteBinaryAssertions) WithVersion(expected string) *remoteBinaryAssertions {
	r.testing.Helper()
	actual, err := r.remoteHost.Execute(fmt.Sprintf("& \"%s\" %s", r.binaryPath, "version"))
	r.NoError(err)
	expectedVersion, err := version.New(expected, "")
	r.NoErrorf(err, "invalid expected version %s", expected)
	actualVersion, err := version.New(actual, "")
	r.NoErrorf(err, "invalid actual version %s", actual)
	r.Equal(expectedVersion.GetNumberAndPre(), actualVersion.GetNumberAndPre(), "version mismatch")
	return r
}
