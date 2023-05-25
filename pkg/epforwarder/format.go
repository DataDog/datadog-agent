// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package epforwarder

import (
	"fmt"

	"github.com/DataDog/agent-payload/v5/contimage"
	"github.com/DataDog/agent-payload/v5/contlcycle"
	"github.com/DataDog/agent-payload/v5/sbom"
	"google.golang.org/protobuf/proto"

	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

type epFormatter struct{}

func (e *epFormatter) Format(m *message.Message, eventType string, redactedMsg []byte) string {
	// TODO Need to do this a better way, but I just want something working first
	output := fmt.Sprintf("type: %v | ", eventType)

	switch eventType {
	case EventTypeContainerLifecycle:
		var msg contlcycle.EventsPayload
		proto.Unmarshal(m.Content, &msg)
		output += msg.String()
	case EventTypeContainerImages:
		var msg contimage.ContainerImagePayload
		proto.Unmarshal(m.Content, &msg)
		output += msg.String()
	case EventTypeContainerSBOM:
		var msg sbom.SBOMPayload
		proto.Unmarshal(m.Content, &msg)
		output += msg.String()
	default:
		output += "UNKNOWN"
	}
	output += "\n"
	return output
}
