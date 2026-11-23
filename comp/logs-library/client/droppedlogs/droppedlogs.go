// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package droppedlogs attributes logs a destination or sender gave up on to their
// source and service, for the dropped-logs Agent Health issue. It lives apart from
// package client so importers of client (the OTel logs exporter among them) do not
// pull in the telemetry stack.
package droppedlogs

import (
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const unknownOrigin = "unknown"

// Target is the one rule for naming a destination in the dropped-logs issue.
// Senders build reliable destinations in GetReliableEndpoints order, which starts
// with the main endpoint, so reliable destination 0 is the main endpoint unless it
// is a Multi-Region Failover one. HTTP, TCP and the sender worker all call this.
func Target(host string, reliable bool, index int, mrf bool) metrics.DropTarget {
	return metrics.DropTarget{Host: host, Main: reliable && index == 0 && !mrf}
}

// BeforeSending names logs dropped before any destination got them. They are
// lost to the main endpoint too.
var BeforeSending = metrics.DropTarget{Main: true}

// RecordPayload attributes every log line of a dropped payload to its source
// and service for the dropped-logs health issue. It does not touch the COAT counters:
// callers keep incrementing TlmLogsDropped exactly as before.
func RecordPayload(payload *message.Payload, reason metrics.DropReason, target metrics.DropTarget) {
	if payload == nil {
		return
	}
	RecordMessages(payload.MessageMetas, reason, target)
}

// RecordMessages is RecordPayload for messages not yet in a payload.
//
// Only log lines count. The event platform forwarder (DBM, NDM, network path, ...)
// shares these senders and destinations, but its messages carry no Origin, so they
// are skipped here and never surface as lost logs.
func RecordMessages(metas []*message.MessageMetadata, reason metrics.DropReason, target metrics.DropTarget) {
	var drops map[metrics.DropOrigin]metrics.DropCount
	for _, meta := range metas {
		if meta == nil || meta.Origin == nil || meta.Origin.LogSource == nil || meta.Origin.LogSource.Config == nil {
			continue
		}
		if drops == nil {
			drops = make(map[metrics.DropOrigin]metrics.DropCount)
		}
		origin := dropOrigin(meta.Origin)
		count := drops[origin]
		count.Logs++
		count.Bytes += int64(meta.RawDataLen)
		drops[origin] = count
	}
	metrics.RecordDroppedLogs(reason, target, drops)
}

// dropOrigin mirrors the rotation-loss identity (pkg/logs/tailers/file): a config
// naming only an integration reports under it rather than "unknown", and the
// service falls back to the source.
func dropOrigin(o *message.Origin) metrics.DropOrigin {
	source := o.Source()
	if source == "" {
		source = o.LogSource.Config.IntegrationName
	}
	service := o.Service()
	if service == "" {
		service = source
	}
	if source == "" {
		source = unknownOrigin
	}
	if service == "" {
		service = unknownOrigin
	}
	return metrics.DropOrigin{Source: source, Service: service}
}
