package metadata

import (
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/common"
	"github.com/stretchr/testify/assert"
	"testing"
)

func Test_batchPayloads(t *testing.T) {
	collectTime := common.MockTimeNow()
	deviceID := "123"
	devices := []DeviceMetadata{
		{ID: deviceID},
	}

	var interfaces []InterfaceMetadata
	for i := 0; i < 350; i++ {
		interfaces = append(interfaces, InterfaceMetadata{DeviceID: deviceID, Index: int32(i)})
	}
	var ipAddresses []IPAddressMetadata
	for i := 0; i < 100; i++ {
		ipAddresses = append(ipAddresses, IPAddressMetadata{InterfaceID: deviceID + ":1", IPAddress: "1.2.3.4", Prefixlen: 24})
	}
	var topologyLinks []TopologyLinkMetadata
	for i := 0; i < 100; i++ {
		topologyLinks = append(topologyLinks, TopologyLinkMetadata{
			Local:  &TopologyLinkSide{Interface: &TopologyLinkInterface{ID: "a"}},
			Remote: &TopologyLinkSide{Interface: &TopologyLinkInterface{ID: "b"}},
		})
	}
	payloads := BatchPayloads("my-ns", "127.0.0.0/30", collectTime, 100, devices, interfaces, ipAddresses, topologyLinks)

	assert.Equal(t, 6, len(payloads))

	assert.Equal(t, "my-ns", payloads[0].Namespace)
	assert.Equal(t, "127.0.0.0/30", payloads[0].Subnet)
	assert.Equal(t, int64(946684800), payloads[0].CollectTimestamp)
	assert.Equal(t, devices, payloads[0].Devices)
	assert.Equal(t, 99, len(payloads[0].Interfaces))
	assert.Equal(t, interfaces[0:99], payloads[0].Interfaces)

	assert.Equal(t, "127.0.0.0/30", payloads[1].Subnet)
	assert.Equal(t, int64(946684800), payloads[1].CollectTimestamp)
	assert.Equal(t, 0, len(payloads[1].Devices))
	assert.Equal(t, 100, len(payloads[1].Interfaces))
	assert.Equal(t, interfaces[99:199], payloads[1].Interfaces)

	assert.Equal(t, 0, len(payloads[2].Devices))
	assert.Equal(t, 100, len(payloads[2].Interfaces))
	assert.Equal(t, interfaces[199:299], payloads[2].Interfaces)

	assert.Equal(t, 0, len(payloads[3].Devices))
	assert.Equal(t, 51, len(payloads[3].Interfaces))
	assert.Equal(t, 49, len(payloads[3].IPAddresses))
	assert.Equal(t, interfaces[299:350], payloads[3].Interfaces)
	assert.Equal(t, ipAddresses[:49], payloads[3].IPAddresses)

	assert.Equal(t, 0, len(payloads[4].Devices))
	assert.Equal(t, 51, len(payloads[4].IPAddresses))
	assert.Equal(t, 49, len(payloads[4].Links))
	assert.Equal(t, ipAddresses[49:], payloads[4].IPAddresses)
	assert.Equal(t, topologyLinks[:49], payloads[4].Links)

	assert.Equal(t, 0, len(payloads[5].Devices))
	assert.Equal(t, 0, len(payloads[5].Interfaces))
	assert.Equal(t, 51, len(payloads[5].Links))
	assert.Equal(t, topologyLinks[49:100], payloads[5].Links)
}
