// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build test

package serverimpl

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameimpl"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/snmptraps/config"
	"github.com/DataDog/datadog-agent/comp/snmptraps/config/configimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/formatter/formatterimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/forwarder/forwarderimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/listener/listenerimpl"
	"github.com/DataDog/datadog-agent/comp/snmptraps/senderhelper"
	"github.com/DataDog/datadog-agent/comp/snmptraps/server"
	"github.com/DataDog/datadog-agent/comp/snmptraps/status/statusimpl"
	ndmtestutils "github.com/DataDog/datadog-agent/pkg/networkdevice/testutils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestStartStop(t *testing.T) {
	freePort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	server := fxutil.Test[server.Component](t,
		log.MockModule,
		configimpl.MockModule,
		formatterimpl.MockModule,
		senderhelper.Opts,
		hostnameimpl.MockModule,
		forwarderimpl.Module,
		statusimpl.MockModule,
		listenerimpl.Module,
		Module,
		fx.Replace(&config.TrapsConfig{
			Enabled:          true,
			Port:             freePort,
			CommunityStrings: []string{"public"},
		}),
	)
	assert.NotEmpty(t, server)
}
