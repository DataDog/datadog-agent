// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package preprocessor

import (
	"strings"

	"github.com/DataDog/datadog-agent/pkg/logs/adaptivesampling"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

func (p *Preprocessor) observeTokenizedLog(msg *message.Message, tokens []Token) {
	if msg == nil || !adaptivesampling.HasTokenizedLogObserver() {
		return
	}

	event := adaptivesampling.TokenizedLogEvent{
		Content:            string(msg.GetContent()),
		Status:             msg.GetStatus(),
		Hostname:           msg.GetHostname(),
		TimestampUnixMilli: msg.GetTimestampUnixMilli(),
		ContainerID:        adaptiveSamplerContainerID(msg),
		Pattern:            TokensToString(tokens),
		PatternHash:        adaptiveSamplerLogHash(tokens),
	}
	if msg.Origin != nil {
		event.Tags = append([]string(nil), msg.Tags()...)
	}
	adaptivesampling.ObserveTokenizedLog(event)
}

func adaptiveSamplerContainerID(msg *message.Message) string {
	if msg == nil || msg.Origin == nil {
		return ""
	}
	if msg.Origin.LogSource != nil && msg.Origin.LogSource.Config != nil && msg.Origin.LogSource.Config.Identifier != "" {
		return msg.Origin.LogSource.Config.Identifier
	}
	if strings.HasPrefix(msg.Origin.Identifier, "docker:") {
		return strings.TrimPrefix(msg.Origin.Identifier, "docker:")
	}
	if strings.HasPrefix(msg.Origin.Identifier, "containerd:") {
		return strings.TrimPrefix(msg.Origin.Identifier, "containerd:")
	}
	return ""
}
