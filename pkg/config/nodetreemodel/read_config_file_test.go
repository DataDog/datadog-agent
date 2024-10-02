// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package nodetreemodel

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

var confYaml = `
network_devices:
  snmp_traps:
    enabled: true
    port: 1234
    community_strings: ["a","b","c"]
    users:
    - user:         alice
      authKey:      hunter2
      authProtocol: MD5
      privKey:      pswd
      privProtocol: AE5
    - user:         bob
      authKey:      "123456"
      authProtocol: MD5
      privKey:      secret
      privProtocol: AE5
    bind_host: ok
    stop_timeout: 4
    namespace: abc
`

func TestReadConfigAndGetValues(t *testing.T) {
	cfg := NewConfig("datadog", "DD", nil)
	err := cfg.ReadConfig(strings.NewReader(confYaml))
	if err != nil {
		panic(err)
	}

	enabled := cfg.GetBool("network_devices.snmp_traps.enabled")
	port := cfg.GetInt("network_devices.snmp_traps.port")
	bindHost := cfg.GetString("network_devices.snmp_traps.bind_host")
	stopTimeout := cfg.GetInt("network_devices.snmp_traps.stop_timeout")
	namespace := cfg.GetString("network_devices.snmp_traps.namespace")

	assert.Equal(t, enabled, true)
	assert.Equal(t, port, 1234)
	assert.Equal(t, bindHost, "ok")
	assert.Equal(t, stopTimeout, 4)
	assert.Equal(t, namespace, "abc")
}
