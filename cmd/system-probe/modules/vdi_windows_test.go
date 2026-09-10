// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package modules

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	vdimodel "github.com/DataDog/datadog-agent/pkg/vdi/model"
)

type fakeVDICollector struct {
	provider string
	result   vdimodel.ProviderInventory
}

func (c fakeVDICollector) Provider() string { return c.provider }

func (c fakeVDICollector) Collect(context.Context) vdimodel.ProviderInventory { return c.result }

func TestVDIInventoryCollectsRegisteredProvidersIndependently(t *testing.T) {
	module := newVDIModule(
		func() ([]vdimodel.WindowsSession, error) {
			return []vdimodel.WindowsSession{{WindowsSessionID: 4, User: "alice", State: "active"}}, nil
		},
		fakeVDICollector{provider: vdimodel.ProviderAWSWorkSpaces, result: vdimodel.ProviderInventory{
			SourceStatus: vdimodel.SourceStatus{Status: vdimodel.StatusError, Error: "dcv unavailable"},
		}},
		fakeVDICollector{provider: "future_provider", result: vdimodel.ProviderInventory{
			SourceStatus: vdimodel.SourceStatus{Status: vdimodel.StatusOK},
		}},
	)

	result := module.inventory(context.Background())
	require.Equal(t, vdimodel.StatusOK, result.Windows.Status)
	require.Equal(t, uint32(4), result.Windows.Sessions[0].WindowsSessionID)
	require.Equal(t, vdimodel.StatusError, result.Providers[vdimodel.ProviderAWSWorkSpaces].Status)
	require.Equal(t, vdimodel.StatusOK, result.Providers["future_provider"].Status)
}

func TestVDIInventoryReportsWTSFailureWithoutDroppingProviders(t *testing.T) {
	module := newVDIModule(
		func() ([]vdimodel.WindowsSession, error) { return nil, errors.New("WTS unavailable") },
		fakeVDICollector{provider: vdimodel.ProviderAWSWorkSpaces, result: vdimodel.ProviderInventory{
			SourceStatus: vdimodel.SourceStatus{Status: vdimodel.StatusOK},
		}},
	)

	result := module.inventory(context.Background())
	require.Equal(t, vdimodel.StatusError, result.Windows.Status)
	require.Equal(t, "WTS unavailable", result.Windows.Error)
	require.Equal(t, vdimodel.StatusOK, result.Providers[vdimodel.ProviderAWSWorkSpaces].Status)
}
