// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package config

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	ipcmock "github.com/DataDog/datadog-agent/comp/core/ipc/mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	emconfig "github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
)

func TestEventStreamEnabledForSupportedKernelsLinux(t *testing.T) {
	t.Setenv("DD_SYSTEM_PROBE_EVENT_MONITORING_NETWORK_PROCESS_ENABLED", strconv.FormatBool(true))
	cfg := configmock.NewSystemProbe(t)
	sysconfig.Adjust(cfg)

	if sysconfig.ProcessEventDataStreamSupported() {
		require.True(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
		emconfig := emconfig.NewConfig()
		secconfig, err := secconfig.NewConfig()
		require.NoError(t, err)

		ipcComp := ipcmock.New(t)

		opts := eventmonitor.Opts{}
		evm, err := eventmonitor.NewEventMonitor(emconfig, secconfig, ipcComp, opts)
		require.NoError(t, err)
		require.NoError(t, evm.Init())
	} else {
		require.False(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
	}
}
