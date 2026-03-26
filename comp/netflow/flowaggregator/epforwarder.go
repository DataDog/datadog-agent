// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/integrations"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/format"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
)

// EPForwarder handles the type-specific flush pipeline: filtering, sequence
// tracking, serialization, and forwarding to the Datadog event platform.
type EPForwarder[T any] interface {
	Submit(result T, ctx common.FlushContext) int
}

// standardEPForwarder implements EPForwarder[FlowBatch].
// It applies TopN filtering, tracks sequence deltas, serializes individual
// flow payloads, and sends exporter metadata.
type standardEPForwarder struct {
	filter      FlowFlushFilter
	seqTracker  *sequenceTracker
	epForwarder eventplatform.Forwarder
	hostname    string
	logger      log.Component
}

var _ EPForwarder[FlowBatch] = (*standardEPForwarder)(nil)

func (s *standardEPForwarder) Submit(flows FlowBatch, ctx common.FlushContext) int {
	// Apply TopN filtering.
	flowsBeforeFilter := len(flows)
	flows = s.filter.Filter(ctx, flows)
	numRowsFiltered := flowsBeforeFilter - len(flows)

	s.logger.Debugf("Flushing %d flows to the forwarder, %d dropped by TopN filtering (flush_duration=%d)",
		len(flows), numRowsFiltered, time.Since(ctx.FlushTime).Milliseconds())

	s.seqTracker.trackAndEmit(flows)

	// TODO: Add flush stats to agent telemetry e.g. aggregator newFlushCountStats()
	if len(flows) > 0 {
		s.sendFlows(flows, ctx.FlushTime)
	}
	sendExporterMetadata(flows, ctx.FlushTime, s.epForwarder, s.logger)
	return len(flows)
}

func (s *standardEPForwarder) sendFlows(flows []*common.Flow, flushTime time.Time) {
	for _, flow := range flows {
		flowPayload := buildPayload(flow, s.hostname, flushTime)

		// Calling MarshalJSON directly as it's faster than calling json.Marshall
		payloadBytes, err := flowPayload.MarshalJSON()
		if err != nil {
			s.logger.Errorf("Error marshalling device metadata: %s", err)
			continue
		}
		s.logger.Tracef("flushed flow: %s", string(payloadBytes))

		m := message.NewMessage(payloadBytes, nil, "", 0)
		err = s.epForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkDevicesNetFlow)
		if err != nil {
			// at the moment, SendEventPlatformEventBlocking can only fail if the event type is invalid
			s.logger.Errorf("Error sending to event platform forwarder: %s", err)
			continue
		}
	}
}

// dedupEPForwarder implements EPForwarder[FlowGroupBatch].
// It collects active reporters for sequence tracking, serializes merged
// payloads (all reporters + ghosts per group), and sends exporter metadata.
type dedupEPForwarder struct {
	seqTracker  *sequenceTracker
	epForwarder eventplatform.Forwarder
	hostname    string
	logger      log.Component
}

var _ EPForwarder[FlowGroupBatch] = (*dedupEPForwarder)(nil)

func (d *dedupEPForwarder) Submit(groups FlowGroupBatch, ctx common.FlushContext) int {
	// Collect active reporters for sequence tracking and exporter metadata.
	// GhostReporters are carry-over metadata snapshots and should not affect
	// sequence delta calculations or exporter registration.
	var activeFlows []*common.Flow
	for _, group := range groups {
		activeFlows = append(activeFlows, group.Reporters...)
	}

	d.logger.Debugf("Flushing %d flow groups (%d active reporters) to the forwarder (flush_duration=%d)",
		len(groups), len(activeFlows), time.Since(ctx.FlushTime).Milliseconds())

	d.seqTracker.trackAndEmit(activeFlows)

	if len(groups) > 0 {
		d.sendMergedFlows(groups, ctx.FlushTime)
	}
	sendExporterMetadata(activeFlows, ctx.FlushTime, d.epForwarder, d.logger)
	// Return total active reporter count so flows_flushed metric is comparable
	// across standard and dedup modes.
	return len(activeFlows)
}

// sendMergedFlows sends one merged event per 5-tuple group. All reporters (active +
// ghost) are included so the platform has the full picture for flow_role assignment.
func (d *dedupEPForwarder) sendMergedFlows(groups []FlowGroup, flushTime time.Time) {
	for _, group := range groups {
		if len(group.Reporters) == 0 {
			continue
		}
		reporters := make([]*common.Flow, 0, len(group.Reporters)+len(group.GhostReporters))
		reporters = append(reporters, group.Reporters...)
		reporters = append(reporters, group.GhostReporters...)
		mergedPayload := buildMergedPayload(reporters, d.hostname, flushTime)
		payloadBytes, err := json.Marshal(mergedPayload)
		if err != nil {
			d.logger.Errorf("Error marshalling merged flow payload: %s", err)
			continue
		}
		d.logger.Tracef("flushed merged flow: %s", string(payloadBytes))

		m := message.NewMessage(payloadBytes, nil, "", 0)
		err = d.epForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkDevicesNetFlow)
		if err != nil {
			d.logger.Errorf("Error sending to event platform forwarder: %s", err)
			continue
		}
	}
}

// sendExporterMetadata builds and sends exporter metadata payloads to the event platform.
func sendExporterMetadata(flows []*common.Flow, flushTime time.Time, epForwarder eventplatform.Forwarder, logger log.Component) {
	// exporterMap structure: map[NAMESPACE]map[EXPORTER_ID]metadata.NetflowExporter
	exporterMap := make(map[string]map[string]metadata.NetflowExporter)

	// orderedExporterIDs is used to build predictable metadata payload (consistent batches and orders)
	// orderedExporterIDs structure: map[NAMESPACE][]EXPORTER_ID
	orderedExporterIDs := make(map[string][]string)

	for _, flow := range flows {
		exporterIPAddress := format.IPAddr(flow.ExporterAddr)
		if exporterIPAddress == "" || strings.HasPrefix(exporterIPAddress, "?") {
			logger.Errorf("Invalid exporter Addr: %s", exporterIPAddress)
			continue
		}
		exporterID := flow.Namespace + ":" + exporterIPAddress + ":" + string(flow.FlowType)
		if _, ok := exporterMap[flow.Namespace]; !ok {
			exporterMap[flow.Namespace] = make(map[string]metadata.NetflowExporter)
		}
		if _, ok := exporterMap[flow.Namespace][exporterID]; ok {
			// this exporter is already in the map, no need to reprocess it
			continue
		}
		exporterMap[flow.Namespace][exporterID] = metadata.NetflowExporter{
			ID:        exporterID,
			IPAddress: exporterIPAddress,
			FlowType:  string(flow.FlowType),
		}
		orderedExporterIDs[flow.Namespace] = append(orderedExporterIDs[flow.Namespace], exporterID)
	}
	for namespace, ids := range orderedExporterIDs {
		var netflowExporters []metadata.NetflowExporter
		for _, exporterID := range ids {
			netflowExporters = append(netflowExporters, exporterMap[namespace][exporterID])
		}
		metadataPayloads := metadata.BatchPayloads(integrations.Netflow, namespace, "", flushTime, metadata.PayloadMetadataBatchSize, nil, nil, nil, nil, nil, netflowExporters, nil)
		for _, payload := range metadataPayloads {
			payloadBytes, err := json.Marshal(payload)
			if err != nil {
				logger.Errorf("Error marshalling device metadata: %s", err)
				continue
			}
			logger.Debugf("netflow exporter metadata payload: %s", string(payloadBytes))
			m := message.NewMessage(payloadBytes, nil, "", 0)
			err = epForwarder.SendEventPlatformEventBlocking(m, eventplatform.EventTypeNetworkDevicesMetadata)
			if err != nil {
				logger.Errorf("Error sending event platform event for netflow exporter metadata: %s", err)
			}
		}
	}
}
