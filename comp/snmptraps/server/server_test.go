// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build test

package server

import (
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/netflow/hostname"
	"github.com/DataDog/datadog-agent/comp/netflow/sender"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
	"github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartStop(t *testing.T) {
	var freePort = testutil.GetFreePort()
	server := fxutil.Test[Component](t,
		log.MockModule,
		config.MockModule,
		formatter.MockModule,
		sender.MockModule,
		hostname.MockModule,
		forwarder.Module,
		status.MockModule,
		listener.Module,
		Module,
		fx.Replace(&config.TrapsConfig{
			Enabled:          true,
			Port:             freePort,
			CommunityStrings: []string{"public"},
		}),
	)
	err := server.Start()
	require.NoError(t, err)
	defer server.Stop()
}
