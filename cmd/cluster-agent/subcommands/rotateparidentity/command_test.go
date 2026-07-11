// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build !windows && kubeapiserver && test

package rotateparidentity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/cmd/cluster-agent/command"
	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	hostnamemock "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/mock"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestRotatePARIdentityCommand(t *testing.T) {
	fxutil.TestOneShotSubcommand(t,
		Commands(&command.GlobalParams{}),
		[]string{"rotate-par-identity"},
		run,
		func() {})
}

func TestRun_DisabledPAR(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.enabled": false,
	})
	hostnameComp, _ := hostnamemock.NewMock("test-host")

	err := run(logmock.New(t), cfg, hostnameComp)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "private_action_runner.enabled is false")
}

func TestRun_K8sSecretIdentityStorageDisabled(t *testing.T) {
	cfg := coreconfig.NewMockWithOverrides(t, map[string]interface{}{
		"private_action_runner.enabled":                 true,
		"private_action_runner.identity_use_k8s_secret": false,
	})
	hostnameComp, _ := hostnamemock.NewMock("test-host")

	err := run(logmock.New(t), cfg, hostnameComp)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "private_action_runner.identity_use_k8s_secret is false")
}
