package metadata

import "time"

func BatchPayloads(namespace string,
	subnet string,
	collectTime time.Time,
	batchSize int,
	devices []DeviceMetadata,
	interfaces []InterfaceMetadata,
	ipAddresses []IPAddressMetadata,
	topologyLinks []TopologyLinkMetadata,
) []NetworkDevicesMetadata {
	var payloads []NetworkDevicesMetadata
	var resourceCount int

	curPayload := NetworkDevicesMetadata{
		Subnet:           subnet,
		Namespace:        namespace,
		CollectTimestamp: collectTime.Unix(),
	}

	for _, deviceMetadata := range devices {
		payloads, curPayload, resourceCount = appendToPayloads(namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.Devices = append(curPayload.Devices, deviceMetadata)
	}

	for _, interfaceMetadata := range interfaces {
		payloads, curPayload, resourceCount = appendToPayloads(namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.Interfaces = append(curPayload.Interfaces, interfaceMetadata)
	}

	for _, ipAddress := range ipAddresses {
		payloads, curPayload, resourceCount = appendToPayloads(namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.IPAddresses = append(curPayload.IPAddresses, ipAddress)
	}

	for _, linkMetadata := range topologyLinks {
		payloads, curPayload, resourceCount = appendToPayloads(namespace, subnet, collectTime, batchSize, resourceCount, payloads, curPayload)
		curPayload.Links = append(curPayload.Links, linkMetadata)
	}
	payloads = append(payloads, curPayload)
	return payloads
}

func appendToPayloads(namespace string, subnet string, collectTime time.Time, batchSize int, resourceCount int, payloads []NetworkDevicesMetadata, payload NetworkDevicesMetadata) ([]NetworkDevicesMetadata, NetworkDevicesMetadata, int) {
	if resourceCount != 0 && resourceCount%batchSize == 0 {
		payloads = append(payloads, payload)
		payload = NetworkDevicesMetadata{
			Subnet:           subnet,
			Namespace:        namespace,
			CollectTimestamp: collectTime.Unix(),
		}
	}
	resourceCount++
	return payloads, payload, resourceCount
}
