// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2020 Datadog, Inc.

package listeners

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestSNMPListener(t *testing.T) {
	newSvc := make(chan Service, 10)
	delSvc := make(chan Service, 10)
	testChan := make(chan snmpSubnet, 10)

	snmpConfig := util.SNMPConfig{
		Network:   "192.168.0.0/24",
		Community: "public",
	}
	listenerConfig := util.SNMPListenerConfig{
		Configs: []util.SNMPConfig{snmpConfig},
	}

	mockConfig := config.Mock()
	mockConfig.Set("snmp_listener", listenerConfig)

	worker = func(l *SNMPListener, jobs <-chan snmpSubnet) {
		subnet := <-jobs
		testChan <- subnet
	}

	l, err := NewSNMPListener()
	assert.Equal(t, nil, err)
	l.Listen(newSvc, delSvc)

	subnet := <-testChan

	assert.Equal(t, "snmp_0", subnet.adIdentifier)
	assert.Equal(t, "192.168.0.0", subnet.currentIP.String())
	assert.Equal(t, "192.168.0.0", subnet.startingIP.String())
	assert.Equal(t, "192.168.0.0/24", subnet.network.String())
	assert.Equal(t, "public", subnet.config.Community)
	assert.Equal(t, "public", subnet.defaultParams.Community)

	subnet = <-testChan
	assert.Equal(t, "192.168.0.1", subnet.currentIP.String())
	assert.Equal(t, "192.168.0.0", subnet.startingIP.String())
}
