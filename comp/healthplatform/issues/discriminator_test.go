// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package issues

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/healthplatform/issueregistry/utils/selfident"
)

// This file runs under whichever selfident variant the build selects: the no-op
// one by default, the Kubernetes one in the kubeapiserver tag set. A SelfIdent
// built with no workloadmeta resolves no DaemonSet either way, so the fallback
// assertions below hold for both — see discriminator_kubeapiserver_test.go for
// the case only the real implementation can exercise.
func TestIssueDiscriminator_FallsBackToHostID(t *testing.T) {
	assert.Equal(t, "some-host-id", IssueDiscriminator(selfident.New(nil), "some-host-id"))
}

func TestIssueDiscriminator_FallsBackToOSHostnameWithoutHostID(t *testing.T) {
	osHostname, err := os.Hostname()
	require.NoError(t, err)

	assert.Equal(t, osHostname, IssueDiscriminator(selfident.New(nil), ""))
}

// A nil SelfIdent must degrade to per-host scoping rather than panic: ModuleDeps
// is a plain struct and callers do leave the field unset. Without the guard this
// passes on non-Kubernetes builds and panics on Kubernetes ones, which is the
// worst way to find out.
func TestIssueDiscriminator_NilSelfIdentFallsBackToHostID(t *testing.T) {
	assert.Equal(t, "some-host-id", IssueDiscriminator(nil, "some-host-id"))
}
