// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package snmpscanimpl

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

func (s snmpScannerImpl) sendPayload(payload metadata.NetworkDevicesMetadata) error {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		s.log.Errorf("Error marshalling device metadata: %v", err)
		return nil
	}
	m := message.NewMessage(payloadBytes, nil, "", 0)
	s.log.Debugf("Device metadata payload is %d bytes", len(payloadBytes))
	s.log.Tracef("Device metadata payload: %s", string(payloadBytes))
	if err := s.epforwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkDevicesMetadata); err != nil {
		return err
	}
	return nil
}
