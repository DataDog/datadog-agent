// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !linux

package systemd

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/rshell/builtins"
)

func TestSystemdManagerMethodsFailClosedOffLinux(t *testing.T) {
	client := NewClient(Target{})
	ctx := context.Background()

	states, err := client.ListSystemServices(ctx, builtins.SystemServiceListRequest{Services: []string{"api.service"}})
	assert.Nil(t, states)
	require.ErrorIs(t, err, builtins.ErrSystemdUnsupported)

	states, err = client.InspectSystemServices(ctx, []string{"api.service"})
	assert.Nil(t, states)
	require.ErrorIs(t, err, builtins.ErrSystemdUnsupported)

	controllerCalls := []struct {
		name string
		call func() error
	}{
		{
			name: "run jobs",
			call: func() error {
				return client.RunSystemServiceJobs(ctx, builtins.SystemServiceJobStart, []string{"api.service"})
			},
		},
		{
			name: "enable",
			call: func() error {
				return client.EnableSystemServices(ctx, []string{"api.service"})
			},
		},
		{
			name: "disable",
			call: func() error {
				return client.DisableSystemServices(ctx, []string{"api.service"})
			},
		},
	}
	for _, test := range controllerCalls {
		t.Run(test.name, func(t *testing.T) {
			require.ErrorIs(t, test.call(), builtins.ErrSystemdUnsupported)
		})
	}
}
