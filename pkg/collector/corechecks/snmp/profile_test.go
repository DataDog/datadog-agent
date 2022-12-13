// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package snmp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/version"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
)

func TestProfileMetadata_f5(t *testing.T) {
	timeNow = common.MockTimeNow
	aggregator.NewBufferedAggregator(nil, nil, "", 1*time.Hour)
	invalidPath, _ := filepath.Abs(filepath.Join("internal", "test", "metadata.d"))
	config.Datadog.Set("confd_path", invalidPath)

	sess := session.CreateMockSession()
	sessionFactory := func(*checkconfig.CheckConfig) (session.Session, error) {
		return sess, nil
	}
	chk := Check{sessionFactory: sessionFactory}
	// language=yaml
	rawInstanceConfig := []byte(`
ip_address: 1.2.3.4
community_string: public
profile: f5-big-ip
oid_batch_size: 20
namespace: profile-metadata
collect_topology: true
`)
	// language=yaml
	rawInitConfig := []byte(`
profiles:
  f5-big-ip:
    definition_file: f5-big-ip.yaml
`)

	err := chk.Configure(integration.FakeConfigHash, rawInstanceConfig, rawInitConfig, "test")
	assert.NoError(t, err)

	sender := mocksender.NewMockSender(chk.ID()) // required to initiate aggregator
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("ServiceCheck", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()

	packet := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.3.6.1.2.1.1.1.0",
				Type:  gosnmp.OctetString,
				Value: []byte("BIG-IP Virtual Edition : Linux 3.10.0-862.14.4.el7.ve.x86_64 : BIG-IP software release 15.0.1, build 0.0.11"),
			},
			{
				Name:  "1.3.6.1.2.1.1.2.0",
				Type:  gosnmp.ObjectIdentifier,
				Value: "1.2.3.4",
			},
			{
				Name:  "1.3.6.1.2.1.1.3.0",
				Type:  gosnmp.TimeTicks,
				Value: 20,
			},
			{
				Name:  "1.3.6.1.2.1.1.5.0",
				Type:  gosnmp.OctetString,
				Value: []byte("foo_sys_name"),
			},
			{
				Name:  "1.3.6.1.2.1.1.6.0",
				Type:  gosnmp.OctetString,
				Value: []byte("paris"),
			},
			{
				Name:  "1.3.6.1.4.1.3375.2.1.3.3.3.0",
				Type:  gosnmp.OctetString,
				Value: []byte("a-serial-num"),
			},
			{
				Name:  "1.3.6.1.4.1.3375.2.1.4.1.0",
				Type:  gosnmp.OctetString,
				Value: []byte("BIG-IP"),
			},
			{
				Name:  "1.3.6.1.4.1.3375.2.1.4.2.0",
				Type:  gosnmp.OctetString,
				Value: []byte("15.0.1"),
			},
			{
				Name:  "1.3.6.1.4.1.3375.2.1.4.4.0",
				Type:  gosnmp.OctetString,
				Value: []byte("Final"),
			},
			{
				Name: "1.3.6.1.4.1.3375.2.1.4.999999.0",
				Type: gosnmp.NoSuchObject,
			},
			{
				Name:  "1.3.6.1.4.1.3375.2.1.6.1.0",
				Type:  gosnmp.OctetString,
				Value: []byte("Linux"),
			},
			{
				Name:  "1.3.6.1.4.1.3375.2.1.6.2.0",
				Type:  gosnmp.OctetString,
				Value: []byte("my-linux-f5-server"),
			},
			{
				Name:  "1.3.6.1.4.1.3375.2.1.6.4.0",
				Type:  gosnmp.OctetString,
				Value: []byte("3.10.0-862.14.4.el7.ve.x86_64"),
			},
		},
	}

	bulkPacket := gosnmp.SnmpPacket{
		Variables: []gosnmp.SnmpPDU{
			{
				Name:  "1.0.8802.1.1.2.1.3.7.1.2.101",
				Type:  gosnmp.Integer,
				Value: 3, // 3->macAddress
			},
			{
				Name:  "1.0.8802.1.1.2.1.3.7.1.3.101",
				Type:  gosnmp.OctetString,
				Value: []byte{0x01, 0x00, 0x00, 0x00, 0x01, 0x03},
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.10.0.101.1",
				Type:  gosnmp.OctetString,
				Value: []byte("RemoteDev1-Description"),
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.4.0.101.1", // chassis id type
				Type:  gosnmp.Integer,
				Value: 4, // 4->macAddress
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.5.0.101.1", // chassis id
				Type:  gosnmp.OctetString,
				Value: []byte{0x01, 0x00, 0x00, 0x00, 0x01, 0x02},
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.6.0.101.1",
				Type:  gosnmp.Integer,
				Value: 3, // 3->macAddress
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.7.0.101.1",
				Type:  gosnmp.OctetString,
				Value: []byte{0x01, 0x00, 0x00, 0x00, 0x01, 0x01},
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.8.0.101.1",
				Type:  gosnmp.OctetString,
				Value: []byte("RemoteDev1-Port1-Description"),
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.9.0.101.1",
				Type:  gosnmp.OctetString,
				Value: []byte("RemoteDev1-Name"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.13.1",
				Type:  gosnmp.Integer,
				Value: 131,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.14.1",
				Type:  gosnmp.Integer,
				Value: 141,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.2.1",
				Type:  gosnmp.OctetString,
				Value: []byte("ifDesc1"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.6.1",
				Type:  gosnmp.OctetString,
				Value: []byte{0x82, 0xa5, 0x6e, 0xa5, 0xc9, 0x01},
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.7.1",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.8.1",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.1.1",
				Type:  gosnmp.OctetString,
				Value: []byte("nameRow1"),
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.18.1",
				Type:  gosnmp.OctetString,
				Value: []byte("descRow1"),
			},
			{
				Name:  "1.0.8802.1.1.2.1.3.7.1.2.102",
				Type:  gosnmp.Integer,
				Value: 3, // 3->macAddress
			},
			{
				Name:  "1.0.8802.1.1.2.1.3.7.1.3.102",
				Type:  gosnmp.OctetString,
				Value: []byte{0x01, 0x00, 0x00, 0x00, 0x02, 0x03},
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.10.0.102.2",
				Type:  gosnmp.OctetString,
				Value: []byte("RemoteDev2-Description"),
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.4.0.102.2",
				Type:  gosnmp.Integer,
				Value: 4, // 4->macAddress
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.5.0.102.2",
				Type:  gosnmp.OctetString,
				Value: []byte{0x01, 0x00, 0x00, 0x00, 0x02, 0x02},
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.6.0.102.2",
				Type:  gosnmp.Integer,
				Value: 3, // 3->macAddress
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.7.0.102.2",
				Type:  gosnmp.OctetString,
				Value: []byte{0x01, 0x00, 0x00, 0x00, 0x02, 0x01},
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.8.0.102.2",
				Type:  gosnmp.OctetString,
				Value: []byte("RemoteDev2-Port1-Description"),
			},
			{
				Name:  "1.0.8802.1.1.2.1.4.1.1.9.0.102.2",
				Type:  gosnmp.OctetString,
				Value: []byte("RemoteDev2-Name"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.13.2",
				Type:  gosnmp.Integer,
				Value: 132,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.14.2",
				Type:  gosnmp.Integer,
				Value: 142,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.2.2",
				Type:  gosnmp.OctetString,
				Value: []byte("ifDesc2"),
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.6.2",
				Type:  gosnmp.OctetString,
				Value: []byte{0x82, 0xa5, 0x6e, 0xa5, 0xc9, 0x02},
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.7.2",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.2.2.1.8.2",
				Type:  gosnmp.Integer,
				Value: 1,
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.1.2",
				Type:  gosnmp.OctetString,
				Value: []byte("nameRow2"),
			},
			{
				Name:  "1.3.6.1.2.1.31.1.1.1.18.2",
				Type:  gosnmp.OctetString,
				Value: []byte("descRow2"),
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
			{
				Name:  "9", // exit table
				Type:  gosnmp.Integer,
				Value: 999,
			},
		},
	}

	sess.On("GetNext", []string{"1.0"}).Return(&gosnmplib.MockValidReachableGetNextPacket, nil)
	sess.On("Get", []string{
		"1.3.6.1.2.1.1.1.0",
		"1.3.6.1.2.1.1.2.0",
		"1.3.6.1.2.1.1.3.0",
		"1.3.6.1.2.1.1.5.0",
		"1.3.6.1.2.1.1.6.0",
		"1.3.6.1.4.1.3375.2.1.3.3.3.0",
		"1.3.6.1.4.1.3375.2.1.4.1.0",
		"1.3.6.1.4.1.3375.2.1.4.2.0",
		"1.3.6.1.4.1.3375.2.1.4.4.0",
		"1.3.6.1.4.1.3375.2.1.4.999999.0",
		"1.3.6.1.4.1.3375.2.1.6.1.0",
		"1.3.6.1.4.1.3375.2.1.6.2.0",
		"1.3.6.1.4.1.3375.2.1.6.4.0",
	}).Return(&packet, nil)
	sess.On("GetBulk", []string{
		"1.0.8802.1.1.2.1.3.7.1.2",
		"1.0.8802.1.1.2.1.3.7.1.3",
		"1.0.8802.1.1.2.1.4.1.1.10",
		"1.0.8802.1.1.2.1.4.1.1.4",
		"1.0.8802.1.1.2.1.4.1.1.5",
		"1.0.8802.1.1.2.1.4.1.1.6",
		"1.0.8802.1.1.2.1.4.1.1.7",
		"1.0.8802.1.1.2.1.4.1.1.8",
		"1.0.8802.1.1.2.1.4.1.1.9",
		"1.3.6.1.2.1.2.2.1.13",
		"1.3.6.1.2.1.2.2.1.14",
		"1.3.6.1.2.1.2.2.1.2",
		"1.3.6.1.2.1.2.2.1.6",
		"1.3.6.1.2.1.2.2.1.7",
		"1.3.6.1.2.1.2.2.1.8",
		"1.3.6.1.2.1.31.1.1.1.1",
		"1.3.6.1.2.1.31.1.1.1.18",
	}, checkconfig.DefaultBulkMaxRepetitions).Return(&bulkPacket, nil)

	err = chk.Run()
	assert.Nil(t, err)

	// language=json
	event := []byte(fmt.Sprintf(`
{
  "subnet": "",
  "namespace":"profile-metadata",
  "devices": [
    {
      "id": "profile-metadata:1.2.3.4",
      "id_tags": [
        "device_namespace:profile-metadata",
        "snmp_device:1.2.3.4"
      ],
      "tags": [
        "agent_version:%s",
        "device_namespace:profile-metadata",
        "device_vendor:f5",
        "snmp_device:1.2.3.4",
        "snmp_host:foo_sys_name",
        "snmp_profile:f5-big-ip"
      ],
      "ip_address": "1.2.3.4",
      "status": 1,
      "name": "foo_sys_name",
      "description": "BIG-IP Virtual Edition : Linux 3.10.0-862.14.4.el7.ve.x86_64 : BIG-IP software release 15.0.1, build 0.0.11",
      "sys_object_id": "1.2.3.4",
      "location": "paris",
      "profile": "f5-big-ip",
      "vendor": "f5",
      "serial_number": "a-serial-num",
      "version":"15.0.1",
      "product_name":"BIG-IP",
      "model":"Final",
      "os_name":"LINUX (3.10.0-862.14.4.el7.ve.x86_64)",
      "os_version":"3.10.0-862.14.4.el7.ve.x86_64",
      "os_hostname":"my-linux-f5-server"
    }
  ],
  "interfaces": [
    {
      "device_id": "profile-metadata:1.2.3.4",
      "id_tags": ["interface:nameRow1"],
      "index": 1,
      "name": "nameRow1",
      "alias": "descRow1",
      "description": "ifDesc1",
      "mac_address": "82:a5:6e:a5:c9:01",
      "admin_status": 1,
      "oper_status": 1
    },
    {
      "device_id": "profile-metadata:1.2.3.4",
	  "id_tags": ["interface:nameRow2"],
      "index": 2,
      "name": "nameRow2",
      "alias": "descRow2",
      "description": "ifDesc2",
      "mac_address": "82:a5:6e:a5:c9:02",
      "admin_status": 1,
      "oper_status": 1
    }
  ],
  "links": [
        {
            "local": {
                "device": {
                    "id": "profile-metadata:1.2.3.4",
                    "id_type": "ndm"
                },
                "interface": {
                    "id": "01:00:00:00:01:03",
                    "id_type": "mac_address"
                }
            },
            "remote": {
                "device": {
                    "id": "01:00:00:00:01:02",
                    "id_type": "mac_address",
                    "name": "RemoteDev1-Name",
                    "description": "RemoteDev1-Description"
                },
                "interface": {
                    "id": "01:00:00:00:01:01",
                    "id_type": "mac_address",
                    "description": "RemoteDev1-Port1-Description"
                }
            }
        },
        {
            "local": {
                "device": {
                    "id": "profile-metadata:1.2.3.4",
                    "id_type": "ndm"
                },
                "interface": {
                    "id": "01:00:00:00:02:03",
                    "id_type": "mac_address"
                }
            },
            "remote": {
                "device": {
                    "id": "01:00:00:00:02:02",
                    "id_type": "mac_address",
                    "name": "RemoteDev2-Name",
                    "description": "RemoteDev2-Description"
                },
                "interface": {
                    "id": "01:00:00:00:02:01",
                    "id_type": "mac_address",
                    "description": "RemoteDev2-Port1-Description"
                }
            }
        }
  ],
  "collect_timestamp":946684800
}
`, version.AgentVersion))
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.String(), "network-devices-metadata")
}
