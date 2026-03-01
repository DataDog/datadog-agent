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

// NewDatagramDecoder builds a decoder for messages arriving as discrete
// datagrams (UDP or Unix datagram).
//
// For syslog format, each datagram is one complete syslog message (RFC 5426),
// so NoFraming is used with the syslog parser. A NoopLineHandler prevents
// truncation logic from corrupting structured messages.
//
// For unstructured format, datagrams may contain multiple newline-separated
// messages, so UTF-8 newline framing is used. The tailer must ensure each
// datagram ends with a newline for prompt frame emission.
func NewDatagramDecoder(source *sources.ReplaceableSource, tailerInfo *status.InfoRegistry) Decoder {
	format := source.Config().Format
	switch format {
	case config.SyslogFormat:
		return newSyslogDatagramDecoder(source, tailerInfo)
	default:
		return InitializeDecoder(source, noop.New(), tailerInfo)
	}
}

func newSyslogDatagramDecoder(source *sources.ReplaceableSource, tailerInfo *status.InfoRegistry) Decoder {
	maxMessageSize := source.Config().GetMaxMessageSizeBytes(pkgconfigsetup.Datadog())
	inputChan := make(chan *message.Message)
	outputChan := make(chan *message.Message)
	detectedPattern := &DetectedPattern{}

	lineHandler := NewNoopLineHandler(outputChan)
	lineParser := NewSingleLineParser(lineHandler, syslogparser.NewFileParser())
	f := framer.NewFramer(lineParser.process, framer.NoFraming, maxMessageSize)

	formatInfo := status.NewMappedInfo("Format")
	formatInfo.SetMessage("Format", config.SyslogFormat)
	tailerInfo.Register(formatInfo)

	return New(inputChan, outputChan, f, lineParser, lineHandler, detectedPattern)
}
