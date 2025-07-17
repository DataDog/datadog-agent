// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package report

import (
	"encoding/json"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	devicemetadata "github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// TimeNow useful for mocking
var TimeNow = time.Now

// SendMetadata send Versa device, interface and IP Address metadata
func (s *Sender) SendMetadata(devices []devicemetadata.DeviceMetadata, interfaces []devicemetadata.InterfaceMetadata, ipAddresses []devicemetadata.IPAddressMetadata) {
	collectionTime := TimeNow()
	metadataPayloads := devicemetadata.BatchPayloads(integrations.Versa, s.namespace, "", collectionTime, devicemetadata.PayloadMetadataBatchSize, devices, interfaces, ipAddresses, nil, nil, nil, nil)
	for _, payload := range metadataPayloads {
		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			log.Errorf("Error marshalling Versa metadata : %s", err)
			continue
		}
		s.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkDevicesMetadata)
	}
}
