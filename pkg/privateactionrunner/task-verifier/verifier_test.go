// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package taskverifier

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/config/setup"
	privateactionspb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/privateactionrunner/privateactions"
)

func TestRemoteActionAllowedPathsByEnv_MapsEnvironments(t *testing.T) {
	got := remoteActionAllowedPathsByEnv([]*privateactionspb.RemoteActionPathRule{
		{Environment: privateactionspb.RemoteActionEnvironment_REMOTE_ACTION_ENVIRONMENT_BARE_METAL, Paths: []string{"/var/log"}},
		{Environment: privateactionspb.RemoteActionEnvironment_REMOTE_ACTION_ENVIRONMENT_CONTAINERIZED, Paths: []string{"/host/var/log"}},
	})

	assert.Equal(t, map[string][]string{
		setup.RShellPathAllowMapDefaultKey:       {"/var/log"},
		setup.RShellPathAllowMapContainerizedKey: {"/host/var/log"},
	}, got)
}

func TestRemoteActionAllowedPathsByEnv_DropsUnknownEnvironment(t *testing.T) {
	got := remoteActionAllowedPathsByEnv([]*privateactionspb.RemoteActionPathRule{
		{Environment: privateactionspb.RemoteActionEnvironment_REMOTE_ACTION_ENVIRONMENT_UNSPECIFIED, Paths: []string{"/whatever"}},
		{Environment: privateactionspb.RemoteActionEnvironment_REMOTE_ACTION_ENVIRONMENT_BARE_METAL, Paths: []string{"/var/log"}},
	})

	assert.Equal(t, map[string][]string{setup.RShellPathAllowMapDefaultKey: {"/var/log"}}, got)
}

func TestRemoteActionAllowedPathsByEnv_EmptyReturnsNil(t *testing.T) {
	assert.Nil(t, remoteActionAllowedPathsByEnv(nil))
	assert.Nil(t, remoteActionAllowedPathsByEnv([]*privateactionspb.RemoteActionPathRule{}))
}
