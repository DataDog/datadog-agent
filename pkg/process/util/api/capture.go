// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unsafe"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// Only producer headers needed to interpret a complete group are observed.
// Authorization, API keys, destination URLs and arbitrary headers are excluded.
var captureGroupHeaders = [...]string{
	headers.HostHeader, headers.ProcessVersionHeader, headers.TimestampHeader,
	headers.ContentTypeHeader, headers.ContentEncodingHeader, headers.RequestIDHeader,
	headers.AgentStartTime, headers.ContainerCountHeader, headers.PayloadSource,
	headers.ProcessesEnabled, headers.ServiceDiscoveryEnabled,
}

// ObserveCaptureGroup owns one ordered group after production queue admission.
// It does no decoding, encoding, I/O or logging, and never returns a submission
// error. The coordinator must validate the copied group off the producer path.
func ObserveCaptureGroup(manager *telemetrycapture.Manager, stream telemetrycapture.Stream, collectedAt time.Time, cadence time.Duration, count int, chunkAt func(int) ([]byte, http.Header)) {
	control, selected := manager.Selected(stream)
	if !selected {
		return
	}
	var reservation *telemetrycapture.Reservation
	defer func() {
		if recover() != nil {
			_ = manager.Fail(control)
		}
		reservation.Discard()
	}()
	if (stream != telemetrycapture.Processes && stream != telemetrycapture.Connections) || count <= 0 || int64(count) > telemetrycapture.MaxItemBytes/int64(unsafe.Sizeof(telemetrycapture.Chunk{})) {
		_ = manager.Fail(control)
		return
	}
	reservation = manager.BeginFor(control, stream, collectedAt, cadence, 256+int64(count)*int64(unsafe.Sizeof(telemetrycapture.Chunk{})))
	if reservation == nil {
		return
	}
	chunks := make([]telemetrycapture.Chunk, count)
	for i := range chunks {
		body, h := chunkAt(i)
		if len(body) == 0 {
			_ = manager.Fail(control)
			return
		}
		size := int64(len(body)) + 256
		for _, key := range captureGroupHeaders {
			values := h.Values(key)
			if len(values) > 1 {
				_ = manager.Fail(control)
				return
			}
			if len(values) == 1 {
				size += 256 + int64(len(key)+len(values[0]))
			}
		}
		if !reservation.Grow(size) {
			return
		}
		chunks[i].Body = make([]byte, len(body))
		copy(chunks[i].Body, body)
		chunks[i].Headers = make(map[string]string)
		for _, key := range captureGroupHeaders {
			if value := h.Get(key); value != "" {
				chunks[i].Headers[strings.Clone(key)] = strings.Clone(value)
			}
		}
	}
	reservation.Commit(telemetrycapture.Payload{Chunks: chunks})
}

// DecodeCaptureGroup validates complete groups in the coordinator, never in a
// producing serializer/sender. All errors are fixed and contain no telemetry.
// Chunk indices come from the existing request ID's low 14 bits; no collection
// timestamp is reconstructed from integer-second transport headers.
func DecodeCaptureGroup(record *telemetrycapture.Record) ([]model.MessageBody, error) {
	invalid := errors.New("invalid captured process or connection group")
	if record == nil || (record.Stream != telemetrycapture.Processes && record.Stream != telemetrycapture.Connections) || len(record.Payload.Chunks) == 0 || record.CollectedAt.IsZero() {
		return nil, invalid
	}
	messages := make([]model.MessageBody, 0, len(record.Payload.Chunks))
	var groupID int32
	var requestPrefix uint64
	var host string
	for i, chunk := range record.Payload.Chunks {
		message, err := model.DecodeMessage(chunk.Body)
		if err != nil {
			return nil, invalid
		}
		var id, size int32
		var hostname string
		switch body := message.Body.(type) {
		case *model.CollectorProc:
			if record.Stream != telemetrycapture.Processes {
				return nil, invalid
			}
			id, size, hostname = body.GroupId, body.GroupSize, body.HostName
		case *model.CollectorConnections:
			if record.Stream != telemetrycapture.Connections {
				return nil, invalid
			}
			id, size, hostname = body.GroupId, body.GroupSize, body.HostName
		default:
			return nil, invalid
		}
		requestID, err := strconv.ParseUint(chunk.Headers[headers.RequestIDHeader], 10, 64)
		if err != nil || requestID&((1<<14)-1) != uint64(i) || int(size) != len(record.Payload.Chunks) || chunk.Headers[headers.HostHeader] != hostname {
			return nil, invalid
		}
		if i == 0 {
			groupID, requestPrefix, host = id, requestID>>14, hostname
		} else if id != groupID || requestID>>14 != requestPrefix || hostname != host {
			return nil, invalid
		}
		messages = append(messages, message.Body)
	}
	return messages, nil
}
