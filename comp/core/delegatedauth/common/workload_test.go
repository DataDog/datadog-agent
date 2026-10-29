// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package common

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func TestWorkloadTokenRequestValidation(t *testing.T) {
	for _, grants := range [][]string{nil, {""}, {"par enrollment"}, {"par_enrollment", "par_enrollment"}, {"par_enrollment\r\nx-injected: value"}, make([]string, 17)} {
		require.Error(t, (WorkloadTokenRequest{Grants: grants}).Validate())
	}
	for _, thumb := range []string{"invalid", strings.Repeat("A", 43) + "="} {
		require.Error(t, (WorkloadTokenRequest{Grants: []string{"par_enrollment"}, JWKThumbprint: thumb}).Validate())
	}
	// Key requirements belong to each grant's server policy, not all workloads.
	require.NoError(t, (WorkloadTokenRequest{Grants: []string{"another_grant"}}).Validate())
	require.NoError(t, (WorkloadTokenRequest{Grants: []string{"par_enrollment", "another_grant"}, JWKThumbprint: strings.Repeat("A", 43)}).Validate())
}
