package metadata

import "time"

func BatchPayloads(namespace string,
	subnet string,
	collectTime time.Time,
	batchSize int,
	device DeviceMetadata,
	interfaces []InterfaceMetadata,
	ipAddresses []IPAddressMetadata,
	topologyLinks []TopologyLinkMetadata,
) []NetworkDevicesMetadata {
	var payloads []NetworkDevicesMetadata
	var resourceCount int
	payload := NetworkDevicesMetadata{
		Devices: []DeviceMetadata{
			device,
		},
		Subnet:           subnet,
		Namespace:        namespace,
		CollectTimestamp: collectTime.Unix(),
	}
	resourceCount++

	for _, interfaceMetadata := range interfaces {
		if resourceCount == batchSize {
			payloads = append(payloads, payload)
			payload = NetworkDevicesMetadata{
				Subnet:           subnet,
				Namespace:        namespace,
				CollectTimestamp: collectTime.Unix(),
			}
			resourceCount = 0
		}
		resourceCount++
		payload.Interfaces = append(payload.Interfaces, interfaceMetadata)
	}

	for _, ipAddress := range ipAddresses {
		if resourceCount == batchSize {
			payloads = append(payloads, payload)
			payload = NetworkDevicesMetadata{
				Subnet:           subnet,
				Namespace:        namespace,
				CollectTimestamp: collectTime.Unix(),
			}
			resourceCount = 0
		}
		resourceCount++
		payload.IPAddresses = append(payload.IPAddresses, ipAddress)
	}

	for _, linkMetadata := range topologyLinks {
		if resourceCount == batchSize {
			payloads = append(payloads, payload)
			payload = NetworkDevicesMetadata{ // TODO: Avoid duplication
				Subnet:           subnet,
				Namespace:        namespace,
				CollectTimestamp: collectTime.Unix(),
			}
			resourceCount = 0
		}
		resourceCount++
		payload.Links = append(payload.Links, linkMetadata)
	}

	payloads = append(payloads, payload)
	return payloads
}
