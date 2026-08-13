// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed by Datadog (https://www.datadoghq.com/).
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
	result vdimodel.ProviderInventory
}

func (c fakeVDICollector) Collect(context.Context) vdimodel.ProviderInventory { return c.result }

func TestVDIInventoryKeepsIndependentSourceResults(t *testing.T) {
	module := newVDIModule(fakeVDICollector{result: vdimodel.ProviderInventory{
		Provider:     vdimodel.ProviderAWSWorkSpaces,
		SourceStatus: vdimodel.SourceStatus{Status: vdimodel.StatusError, Error: "dcv unavailable"},
	}}, func() ([]vdimodel.WindowsSession, error) {
		return []vdimodel.WindowsSession{{OSSessionID: 4, OSUser: "user", State: "active"}}, nil
	})

	result := module.inventory(context.Background())
	require.Equal(t, vdimodel.StatusOK, result.Windows.Status)
	require.Len(t, result.Windows.Sessions, 1)
	require.Equal(t, vdimodel.StatusError, result.Providers[vdimodel.ProviderAWSWorkSpaces].Status)
}

func TestVDIInventoryReportsWTSFailureWithoutDroppingProvider(t *testing.T) {
	module := newVDIModule(fakeVDICollector{result: vdimodel.ProviderInventory{
		Provider:     vdimodel.ProviderAWSWorkSpaces,
		SourceStatus: vdimodel.SourceStatus{Status: vdimodel.StatusOK},
	}}, func() ([]vdimodel.WindowsSession, error) {
		return nil, errors.New("WTS unavailable")
	})

	result := module.inventory(context.Background())
	require.Equal(t, vdimodel.StatusError, result.Windows.Status)
	require.Equal(t, vdimodel.StatusOK, result.Providers[vdimodel.ProviderAWSWorkSpaces].Status)
}
