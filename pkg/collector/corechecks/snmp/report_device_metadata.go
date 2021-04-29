package snmp

import (
	"github.com/DataDog/agent-payload/network-devices"
	"github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/forwarder"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func (ms *metricSender) reportDeviceMetadata(store *resultValueStore, tags []string) {
	log.Debugf("[DEV] Reporting NetworkDevicesMetadata")

	deviceMessage := &network_devices.Device{
		Id:                  "my-Id",
		Name:                "my-Name",
		Description:         "my-Description",
		IpAddress:           "1.2.3.4",
		AutodiscoverySubnet: "1.2.3.4/28",
		SysObjectId:         "1.2.3.4.5.6.6.7.8",
		Profile:             "my-profile",
		Vendor:              "my-vendor",
		Tags:                tags,
		Interfaces: []*network_devices.Interface{
			{
				Index:       1,
				Name:        "interface-1",
				Alias:       "alias-1",
				Description: "interface-1-desc",
				MacAddress:  "3c:22:fb:40:b4:a1",
			},
			{
				Index:       2,
				Name:        "interface-2",
				Alias:       "alias-2",
				Description: "interface-2-desc",
				MacAddress:  "3c:22:fb:40:b4:a2",
			},
		},
	}

	ms.sendDeviceMetadata(deviceMessage)
}

func (ms *metricSender) sendDeviceMetadata(clusterMessage process.MessageBody) {
	ms.sender.NetworkDevicesMetadata([]serializer.ProcessMessageBody{clusterMessage}, forwarder.PayloadTypeDevice)
	//stats := orchestrator.CheckStats{
	//	CacheHits: 0,
	//	CacheMiss: 1,
	//	NodeType:  orchestrator.K8sCluster,
	//}
	//orchestrator.KubernetesResourceCache.Set(orchestrator.BuildStatsKey(orchestrator.K8sCluster), stats, orchestrator.NoExpiration)
}
