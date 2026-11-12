// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmconnectivitycheckimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/networkdevices/connectivity"
)

func TestScanTagsEachResultWithItsGroup(t *testing.T) {
	c := newImpl(logmock.New(t))

	res, err := c.scan(context.Background(), connectivity.Request{
		Targets: []string{"10.0.0.1", "10.0.0.2"},
		Checks:  []string{},
		TargetGroups: []connectivity.TargetGroup{
			{ID: "group-1", TargetIPs: []string{"10.0.0.2"}},
			{ID: "group-2", TargetIPs: []string{"10.0.0.1"}},
		},
	})

	require.NoError(t, err)
	require.Len(t, res.Devices, 2)
	assert.Equal(t, "10.0.0.1", res.Devices[0].IPAddress, "in the targets' order")
	assert.Equal(t, "group-2", res.Devices[0].GroupID)
	assert.Equal(t, "10.0.0.2", res.Devices[1].IPAddress)
	assert.Equal(t, "group-1", res.Devices[1].GroupID)
}

func TestScanWithoutGroupsLeavesTheGroupEmpty(t *testing.T) {
	c := newImpl(logmock.New(t))

	res, err := c.scan(context.Background(), connectivity.Request{
		Targets: []string{"10.0.0.1"},
		Checks:  []string{},
	})

	require.NoError(t, err)
	require.Len(t, res.Devices, 1)
	assert.Empty(t, res.Devices[0].GroupID)
}

func TestScanRejectsAnInvalidGroupedRequest(t *testing.T) {
	c := newImpl(logmock.New(t))

	_, err := c.scan(context.Background(), connectivity.Request{
		Targets:      []string{"10.0.0.1"},
		Checks:       []string{},
		TargetGroups: []connectivity.TargetGroup{{ID: "group-1", TargetIPs: []string{"10.0.0.2"}}},
	})

	assert.ErrorIs(t, err, connectivity.ErrInvalidRequest)
}
