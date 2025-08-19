// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build test

package serverimpl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/snmptraps/senderhelper"
	"github.com/DataDog/datadog-agent/comp/snmptraps/server"
	ndmtestutils "github.com/DataDog/datadog-agent/pkg/networkdevice/testutils"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestServer(t *testing.T) {
	confdPath, err := os.MkdirTemp("", "trapsdb")
	require.NoError(t, err)
	defer os.RemoveAll(confdPath)
	snmpD := filepath.Join(confdPath, "snmp.d")
	tdb := filepath.Join(snmpD, "traps_db")
	require.NoError(t, os.Mkdir(snmpD, 0777))
	require.NoError(t, os.Mkdir(tdb, 0777))
	require.NoError(t, os.WriteFile(filepath.Join(tdb, "foo.json"), []byte{}, 0666))
	freePort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	server := fxutil.Test[server.Component](t,
		senderhelper.Opts,
		fx.Replace(config.MockParams{
			Overrides: map[string]interface{}{
				"confd_path":                                   confdPath,
				"network_devices.snmp_traps.enabled":           true,
				"network_devices.snmp_traps.port":              freePort,
				"network_devices.snmp_traps.community_strings": []string{"public"},
			},
		}),
		Module(),
	)
	assert.NotEmpty(t, server)
	assert.NoError(t, server.Error())
	assert.True(t, server.Running())
}

func TestNonBlockingFailure(t *testing.T) {
	confdPath, err := os.MkdirTemp("", "trapsdb")
	require.NoError(t, err)
	defer os.RemoveAll(confdPath)
	freePort, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	server := fxutil.Test[server.Component](t,
		senderhelper.Opts,
		fx.Replace(config.MockParams{
			Overrides: map[string]interface{}{
				"confd_path":                                   confdPath,
				"network_devices.snmp_traps.enabled":           true,
				"network_devices.snmp_traps.port":              freePort,
				"network_devices.snmp_traps.community_strings": []string{"public"},
			},
		}),
		Module(),
	)
	assert.NotEmpty(t, server)
	assert.ErrorIs(t, server.Error(), os.ErrNotExist)
	assert.False(t, server.Running())
}

func TestDisabled(t *testing.T) {
	server := fxutil.Test[server.Component](t,
		senderhelper.Opts,
		fx.Replace(config.MockParams{
			Overrides: map[string]interface{}{
				"network_devices.snmp_traps.enabled": false,
			},
		}),
		Module(),
	)
	assert.NotNil(t, server)
	assert.NoError(t, server.Error())
	assert.False(t, server.Running())
}
