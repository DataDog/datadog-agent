// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020-present Datadog, Inc.

//go:build test

package serverimpl

import (
	"hash/fnv"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/snmptraps/senderhelper"
	"github.com/DataDog/datadog-agent/comp/snmptraps/server"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// uniqueTestPort returns a deterministic high-range UDP port derived from inputs,
// reducing the chance of collisions across concurrent tests without TOCTOU races.
func uniqueTestPort(keys ...string) uint16 {
	h := fnv.New32a()
	h.Write([]byte(strings.Join(keys, "|")))
	// Choose from 62000-63999 (typically outside Linux default ephemeral range)
	return uint16(62000 + (h.Sum32() % 2000))
}

func TestServer(t *testing.T) {
	confdPath, err := os.MkdirTemp("", "trapsdb")
	require.NoError(t, err)
	defer os.RemoveAll(confdPath)
	snmpD := filepath.Join(confdPath, "snmp.d")
	tdb := filepath.Join(snmpD, "traps_db")
	require.NoError(t, os.Mkdir(snmpD, 0777))
	require.NoError(t, os.Mkdir(tdb, 0777))
	require.NoError(t, os.WriteFile(filepath.Join(tdb, "foo.json"), []byte{}, 0666))
	// Pick a deterministic port specific to this test run to avoid collisions
	port := uniqueTestPort(t.Name(), confdPath)
	server := fxutil.Test[server.Component](t,
		senderhelper.Opts,
		fx.Provide(func(t testing.TB) config.Component {
			return config.NewMockWithOverrides(t, map[string]interface{}{
				"confd_path":                                   confdPath,
				"network_devices.snmp_traps.enabled":           true,
				"network_devices.snmp_traps.port":              port,
				"network_devices.snmp_traps.bind_host":         "127.0.0.1",
				"network_devices.snmp_traps.community_strings": []string{"public"},
			})
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
	port := uniqueTestPort(t.Name(), confdPath)
	server := fxutil.Test[server.Component](t,
		senderhelper.Opts,
		fx.Provide(func(t testing.TB) config.Component {
			return config.NewMockWithOverrides(t, map[string]interface{}{
				"confd_path":                                   confdPath,
				"network_devices.snmp_traps.enabled":           true,
				"network_devices.snmp_traps.port":              port,
				"network_devices.snmp_traps.bind_host":         "127.0.0.1",
				"network_devices.snmp_traps.community_strings": []string{"public"},
			})
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
		fx.Provide(func(t testing.TB) config.Component {
			return config.NewMockWithOverrides(t, map[string]interface{}{
				"network_devices.snmp_traps.enabled": false,
			})
		}),
		Module(),
	)
	assert.NotNil(t, server)
	assert.NoError(t, server.Error())
	assert.False(t, server.Running())
}
