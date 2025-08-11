// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package sender provides a wrapper around the sender.Sender to send network device configuration data
package sender

import (
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	ncmreport "github.com/DataDog/datadog-agent/pkg/networkconfigmanagement/report"
)

// NCMSender is a wrapper around the sender.Sender to send network device configuration data
type NCMSender struct {
	Sender    sender.Sender
	namespace string
}

// NewNCMSender creates a new NCMSender
func NewNCMSender(sender sender.Sender, namespace string) *NCMSender {
	return &NCMSender{
		Sender:    sender,
		namespace: namespace,
	}
}

// SendNCMMetrics sends the network device configuration metrics to Datadog
func (s *NCMSender) SendNCMMetrics() error {
	// Implement the logic to send the network device configuration
	// This is a placeholder implementation
	//s.GaugeWithTimestamp()
	return nil
}

// SendNCMConfig sends the network device configuration payload to event platform
func (s *NCMSender) SendNCMConfig(payload ncmreport.NCMPayload) error {
	payloadBytes, err := json.Marshal(payload)
	fmt.Println(string(payloadBytes))
	if err != nil {
		return err
	}
	s.Sender.EventPlatformEvent(payloadBytes, eventplatform.EventTypeNetworkConfigManagement)
	// TODO: send metrics about the config retrieval?
	return nil
}

// Commit commits the sender (important to ensure data is flushed/sent
func (s *NCMSender) Commit() {
	s.Sender.Commit()
}
