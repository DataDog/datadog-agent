// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package nicinfo

import (
	"net"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGet reads the interfaces of the host, and tests what holds for any Mac.
func TestGet(t *testing.T) {
	infos, err := Get()
	require.NoError(t, err)
	for name, info := range infos {
		builtin := "unknown"
		if info.Builtin != nil {
			builtin = strconv.FormatBool(*info.Builtin)
		}
		t.Logf("%s: %+v builtin=%s", name, info, builtin)
		assert.NotEmpty(t, name)
		if info.PermanentMAC != "" {
			_, err := net.ParseMAC(info.PermanentMAC)
			assert.NoError(t, err, name)
		}
		// Every interface of the registry has a controller, but not every controller has a MAC address
		if info.Builtin == nil {
			assert.Empty(t, info.DriverClass, name)
		} else {
			assert.NotEmpty(t, info.DriverClass, name)
		}
	}
}
