// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRunnerURN_ValidURN(t *testing.T) {
	urn := "urn:dd:apps:on-prem-runner:us1:12345:runner-abc"

	parts, err := ParseRunnerURN(urn)

	require.NoError(t, err)
	assert.Equal(t, "us1", parts.Region)
	assert.Equal(t, int64(12345), parts.OrgID)
	assert.Equal(t, "runner-abc", parts.RunnerID)
}

func TestParseRunnerURN_TooFewSegments(t *testing.T) {
	_, err := ParseRunnerURN("urn:dd:apps:on-prem-runner")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid URN format")
}

func TestParseRunnerURN_TooManySegments(t *testing.T) {
	_, err := ParseRunnerURN("urn:dd:apps:on-prem-runner:us1:12345:runner:extra")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid URN format")
}

func TestParseRunnerURN_NonNumericOrgID(t *testing.T) {
	urn := "urn:dd:apps:on-prem-runner:us1:not-a-number:runner-abc"

	_, err := ParseRunnerURN(urn)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid orgId")
}

func TestMakeRunnerURN_ProducesCorrectFormat(t *testing.T) {
	urn := MakeRunnerURN("eu1", 99999, "runner-xyz")

	assert.Equal(t, "urn:dd:apps:on-prem-runner:eu1:99999:runner-xyz", urn)
}

// TestMakeRunnerURN_ParseRunnerURN_RoundTrip verifies that a URN produced by MakeRunnerURN
// can be parsed back into identical fields by ParseRunnerURN.
func TestMakeRunnerURN_ParseRunnerURN_RoundTrip(t *testing.T) {
	region := "ap1"
	orgID := int64(55555)
	runnerID := "runner-round-trip"

	urn := MakeRunnerURN(region, orgID, runnerID)
	parts, err := ParseRunnerURN(urn)

	require.NoError(t, err)
	assert.Equal(t, region, parts.Region)
	assert.Equal(t, orgID, parts.OrgID)
	assert.Equal(t, runnerID, parts.RunnerID)
}
