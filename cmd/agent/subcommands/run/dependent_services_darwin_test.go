// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package run

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/model"
)

func TestSyncProcmgrWithConfig_Enabled(t *testing.T) {
	var calls [][]string
	orig := launchctl
	t.Cleanup(func() { launchctl = orig })
	launchctl = func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		return nil, nil
	}

	cfg := configmock.New(t)
	cfg.Set("process_manager.enabled", true, model.SourceDefault)
	syncProcmgrWithConfig(cfg)

	require.Equal(t, [][]string{
		{"enable", "system/com.datadoghq.procmgr"},
		{"bootstrap", "system", "/Library/LaunchDaemons/com.datadoghq.procmgr.plist"},
		{"kickstart", "system/com.datadoghq.procmgr"},
	}, calls)
}

func TestSyncProcmgrWithConfig_Disabled(t *testing.T) {
	var calls [][]string
	orig := launchctl
	t.Cleanup(func() { launchctl = orig })
	launchctl = func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		return nil, nil
	}

	cfg := configmock.New(t)
	cfg.Set("process_manager.enabled", false, model.SourceDefault)
	syncProcmgrWithConfig(cfg)

	require.Equal(t, [][]string{
		{"disable", "system/com.datadoghq.procmgr"},
		{"bootout", "system/com.datadoghq.procmgr"},
	}, calls)
}
