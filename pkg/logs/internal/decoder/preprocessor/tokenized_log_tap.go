// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package preprocessor

import (
	"strings"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/logs/adaptivesampling"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

var tokenizedLogTapDebugCount atomic.Uint64

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
	logTokenizedLogTapEvent(msg, event)
	adaptivesampling.ObserveTokenizedLog(event)
}

func logTokenizedLogTapEvent(msg *message.Message, event adaptivesampling.TokenizedLogEvent) {
	count := tokenizedLogTapDebugCount.Add(1)
	if !adaptivesampling.ShouldLogDebugSample(count) {
		return
	}

	var sourceName, sourceType, originID string
	if msg != nil && msg.Origin != nil {
		originID = msg.Origin.Identifier
		if msg.Origin.LogSource != nil {
			sourceName = msg.Origin.LogSource.Name
			sourceType = string(msg.Origin.LogSource.GetSourceType())
		}
	}
	pkglog.Infof("%s preprocessor emitted tokenized log observation count=%d source=%q source_type=%q origin=%q container_id=%q pattern_hash=%q pattern=%q content=%q tag_count=%d",
		adaptivesampling.DebugLogPrefix,
		count,
		sourceName,
		sourceType,
		originID,
		event.ContainerID,
		event.PatternHash,
		adaptivesampling.TruncateDebugString(event.Pattern, 180),
		adaptivesampling.TruncateDebugString(event.Content, 180),
		len(event.Tags))
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
