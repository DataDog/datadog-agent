// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package util

import (
	"testing"

	"github.com/soniah/gosnmp"
	"github.com/stretchr/testify/assert"
)

func TestBuildSNMPParams(t *testing.T) {
	config := SNMPConfig{
		Network: "192.168.0.0/24",
	}
	_, err := config.BuildSNMPParams()
	assert.Equal(t, "No authentication mechanism specified", err.Error())

	config = SNMPConfig{
		Network: "192.168.0.0/24",
		User:    "admin",
		Version: "4",
	}
	_, err = config.BuildSNMPParams()
	assert.Equal(t, "SNMP version not supported: 4", err.Error())

	config = SNMPConfig{
		Network:   "192.168.0.0/24",
		Community: "public",
	}
	params, _ := config.BuildSNMPParams()
	assert.Equal(t, gosnmp.Version2c, params.Version)
	assert.Equal(t, 161, int(params.Port))

	config = SNMPConfig{
		Network: "192.168.0.0/24",
		User:    "admin",
	}
	params, _ = config.BuildSNMPParams()
	assert.Equal(t, gosnmp.Version3, params.Version)
	assert.Equal(t, gosnmp.NoAuthNoPriv, params.MsgFlags)

	config = SNMPConfig{
		Network:      "192.168.0.0/24",
		User:         "admin",
		AuthProtocol: "foo",
	}
	_, err = config.BuildSNMPParams()
	assert.Equal(t, "Unsupported authentication protocol: foo", err.Error())

	config = SNMPConfig{
		Network:      "192.168.0.0/24",
		User:         "admin",
		PrivProtocol: "bar",
	}
	_, err = config.BuildSNMPParams()
	assert.Equal(t, "Unsupported privacy protocol: bar", err.Error())
}
