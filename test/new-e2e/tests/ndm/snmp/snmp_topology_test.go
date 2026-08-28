// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package snmp

import (
	_ "embed"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/test/e2e-framework/components/datadog/agentparams"
	scenec2 "github.com/DataDog/datadog-agent/test/e2e-framework/scenarios/aws/ec2"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/environments"
	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners"
	awshost "github.com/DataDog/datadog-agent/test/e2e-framework/testing/provisioners/aws/host"
	"github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

//go:embed config-vm/cisco-cdp-topology.yaml
var snmpCDPTopologyConfig string

type snmpTopologySuite struct {
	e2e.BaseSuite[environments.Host]
}

func snmpTopologyProvisioner() provisioners.Provisioner {
	return awshost.Provisioner(
		awshost.WithRunOptions(
			scenec2.WithDocker(),
			scenec2.WithAgentOptions(
				agentparams.WithFile("/etc/datadog-agent/conf.d/snmp.d/snmp.yaml", snmpCDPTopologyConfig, true),
			),
		),
	)
}

func TestSnmpTopologySuite(t *testing.T) {
	t.Parallel()
	e2e.Run(t, &snmpTopologySuite{}, e2e.WithProvisioner(snmpTopologyProvisioner()))
}

func linksByID(links []aggregator.TopologyLinkMetadata) map[string]aggregator.TopologyLinkMetadata {
	byID := make(map[string]aggregator.TopologyLinkMetadata, len(links))
	for _, link := range links {
		byID[link.ID] = link
	}
	return byID
}

func linkIDs(links []aggregator.TopologyLinkMetadata) []string {
	ids := make([]string, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.ID)
	}
	return ids
}

func (v *snmpTopologySuite) TestCDPRemoteDeviceName() {
	vm := v.Env().RemoteHost
	fakeIntake := v.Env().FakeIntake

	setupDevice(v.Require(), vm)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		checkBasicMetrics(c, fakeIntake)
	}, 2*time.Minute, 10*time.Second)

	require.EventuallyWithT(v.T(), func(c *assert.CollectT) {
		ndmPayload := checkLastNDMPayload(c, fakeIntake, "default")
		require.Len(c, ndmPayload.Links, 2)

		byID := linksByID(ndmPayload.Links)
		ids := linkIDs(ndmPayload.Links)

		withoutSysName, found := byID["default:127.0.0.1:7.1"]
		require.True(c, found, "no link for the neighbour advertising no cdpCacheSysName, got links %v", ids)
		assert.Equal(c, "cdp", withoutSysName.SourceType)
		assert.Equal(c, "snmp", withoutSysName.Integration)
		assert.Equal(c, "default:127.0.0.1", withoutSysName.Local.Device.DDID)
		assert.Equal(c, "default:127.0.0.1:7", withoutSysName.Local.Interface.DDID)
		assert.Equal(c, "NYC-36FL-AP02", withoutSysName.Remote.Device.ID)
		assert.Equal(c, "NYC-36FL-AP02", withoutSysName.Remote.Device.Name)
		assert.Equal(c, "10.46.6.109", withoutSysName.Remote.Device.IPAddress)
		assert.Equal(c, "GigabitEthernet0", withoutSysName.Remote.Interface.ID)
		assert.Equal(c, "interface_name", withoutSysName.Remote.Interface.IDType)

		withSysName, found := byID["default:127.0.0.1:8.1"]
		require.True(c, found, "no link for the neighbour advertising a cdpCacheSysName, got links %v", ids)
		assert.Equal(c, "FDO12345ABC", withSysName.Remote.Device.ID)
		assert.Equal(c, "spine-01.lab.internal", withSysName.Remote.Device.Name)
	}, 6*time.Minute, 10*time.Second)
}
