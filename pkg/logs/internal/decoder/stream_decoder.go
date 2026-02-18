// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/framer"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	syslogparser "github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/syslog"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// NewStreamDecoder builds a decoder for messages arriving over a
// stream-oriented connection (TCP or Unix stream).
//
// It supports syslog format and unstructured format.
//
// For syslog format, it uses SyslogFraming (RFC 6587 octet counting / non-transparent framing)
// paired with the same syslog parser used for file-based syslog. The parser
// produces StateStructured messages; a NoopLineHandler is used to prevent
// truncation logic from corrupting them.
//
// For unstructured format, it uses UTF-8 newline framing with a noop parser.
func NewStreamDecoder(source *sources.ReplaceableSource, tailerInfo *status.InfoRegistry) Decoder {
	format := source.Config().Format
	switch format {
	case config.SyslogFormat:
		return newSyslogStreamDecoder(source, tailerInfo)
	default:
		return InitializeDecoder(source, noop.New(), tailerInfo)
	}
}

func newSyslogStreamDecoder(source *sources.ReplaceableSource, tailerInfo *status.InfoRegistry) Decoder {
	maxMessageSize := source.Config().GetMaxMessageSizeBytes(pkgconfigsetup.Datadog())
	inputChan := make(chan *message.Message)
	outputChan := make(chan *message.Message)
	detectedPattern := &DetectedPattern{}

	lineHandler := NewNoopLineHandler(outputChan)
	lineParser := NewSingleLineParser(lineHandler, syslogparser.NewFileParser())
	f := framer.NewFramer(lineParser.process, framer.SyslogFraming, maxMessageSize)

	formatInfo := status.NewMappedInfo("Format")
	formatInfo.SetMessage("Format", config.SyslogFormat)
	tailerInfo.Register(formatInfo)

	return New(inputChan, outputChan, f, lineParser, lineHandler, detectedPattern)
}
