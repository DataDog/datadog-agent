// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package api

import (
	"fmt"
	"strconv"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
)

var (
	tlmBytesIn = telemetry.NewCounter("process", "payloads_bytes_in",
		[]string{"type"}, "Count of bytes before encoding payload")
	tlmBytesOut = telemetry.NewCounter("process", "payloads_bytes_out",
		[]string{"type"}, "Count of bytes after encoding payload")
)

// EncodePayload encodes a process message into a payload
func EncodePayload(m model.MessageBody) ([]byte, error) {
	msgType, err := model.DetectMessageType(m)
	if err != nil {
		return nil, fmt.Errorf("unable to detect message type: %s", err)
	}

	typeTag := "type:" + messageTypeToString(msgType)
	tlmBytesIn.Add(float64(m.Size()), typeTag)

	encoded, err := model.EncodeMessage(model.Message{
		Header: model.MessageHeader{
			Version:  model.MessageV3,
			Encoding: model.MessageEncodingZstdPB,
			Type:     msgType,
		}, Body: m})

	tlmBytesOut.Add(float64(len(encoded)), typeTag)

	return encoded, err
}

func messageTypeToString(m model.MessageType) string {
	switch m {
	case model.TypeCollectorProc:
		return "process"
	case model.TypeCollectorConnections:
		return "network"
	case model.TypeCollectorRealTime:
		return "process-rt"
	case model.TypeCollectorContainer:
		return "container"
	case model.TypeCollectorContainerRealTime:
		return "container-rt"
	case model.TypeCollectorPod:
		return "pod"
	case model.TypeCollectorReplicaSet:
		return "replica-set"
	case model.TypeCollectorDeployment:
		return "deployment"
	}
	// otherwise convert the type identifier
	return strconv.Itoa(int(m))
}
