// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package ndmdiscoveryimpl

import (
	"encoding/json"
	"fmt"
	"time"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

// payloadSender is the slice of the event platform forwarder this component
// needs.
type payloadSender interface {
	SendEventPlatformEventBlocking(m *message.Message, eventType string) error
}

// discoveryReporter publishes autodiscovery results to the NDM backend.
type discoveryReporter interface {
	ReportDevices(namespace string, devices []metadata.DiscoveredDeviceMetadata) error
	ReportRun(namespace string, run metadata.AutodiscoveryRunMetadata) error
}

// payloadReporter sends discovery results on the network-devices-metadata
// stream, the same transport every other NDM producer uses.
type payloadReporter struct {
	sender payloadSender
	log    log.Component
	now    func() int64
}

// compile-time assertion that payloadReporter implements discoveryReporter.
// discoveryReporter's consumer is Task 8's sweeper.
var _ discoveryReporter = (*payloadReporter)(nil)

func newPayloadReporter(sender payloadSender, logger log.Component) *payloadReporter {
	return &payloadReporter{
		sender: sender,
		log:    logger,
		now:    func() int64 { return time.Now().Unix() },
	}
}

// ReportDevices sends the probed addresses in batches of
// metadata.PayloadMetadataBatchSize, matching every other NDM producer so the
// existing intake limits hold.
func (r *payloadReporter) ReportDevices(namespace string, devices []metadata.DiscoveredDeviceMetadata) error {
	// One collect time for every batch of a single call, matching
	// metadata.BatchDeviceScan, so the backend sees one coherent snapshot.
	collectTime := r.now()
	for start := 0; start < len(devices); start += metadata.PayloadMetadataBatchSize {
		end := start + metadata.PayloadMetadataBatchSize
		if end > len(devices) {
			end = len(devices)
		}
		payload := metadata.NetworkDevicesMetadata{
			Namespace:         namespace,
			Integration:       integrations.SNMP,
			CollectTimestamp:  collectTime,
			DiscoveredDevices: devices[start:end],
		}
		if err := r.send(payload); err != nil {
			return err
		}
	}
	return nil
}

// ReportRun sends one autodiscovery run lifecycle record.
func (r *payloadReporter) ReportRun(namespace string, run metadata.AutodiscoveryRunMetadata) error {
	return r.send(metadata.NetworkDevicesMetadata{
		Namespace:         namespace,
		Integration:       integrations.SNMP,
		CollectTimestamp:  r.now(),
		AutodiscoveryRuns: []metadata.AutodiscoveryRunMetadata{run},
	})
}

func (r *payloadReporter) send(payload metadata.NetworkDevicesMetadata) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal the discovery payload: %w", err)
	}

	r.log.Debugf("sending network devices discovery payload: %s", string(raw))

	m := message.NewMessage(raw, nil, "", 0)
	if err := r.sender.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkDevicesMetadata); err != nil {
		return fmt.Errorf("failed to send the discovery payload: %w", err)
	}
	return nil
}
