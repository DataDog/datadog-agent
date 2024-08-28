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

	sysconfig "github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/comp/core"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/core/telemetry/telemetryimpl"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	wmmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	emconfig "github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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

		opts := eventmonitor.Opts{}
		telemetry := fxutil.Test[telemetry.Component](t, telemetryimpl.MockModule())
		wmeta := fxutil.Test[workloadmeta.Component](t,
			core.MockBundle(),
			wmmock.MockModule(workloadmeta.NewParams()),
		)
		evm, err := eventmonitor.NewEventMonitor(emconfig, secconfig, opts, wmeta, telemetry)
		require.NoError(t, err)
		require.NoError(t, evm.Init())
	} else {
		require.False(t, cfg.GetBool("event_monitoring_config.network_process.enabled"))
	}
}
