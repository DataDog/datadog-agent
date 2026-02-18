// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"regexp"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/framer"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/dockerfile"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/encodedtext"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/integrations"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	syslogparser "github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/syslog"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
	status "github.com/DataDog/datadog-agent/pkg/logs/status/utils"
)

// NewDecoderFromSource creates a new decoder from a log source
func NewDecoderFromSource(source *sources.ReplaceableSource, tailerInfo *status.InfoRegistry) Decoder {
	return NewDecoderFromSourceWithPattern(source, nil, tailerInfo)
}

// NewDecoderFromSourceWithPattern creates a new decoder from a log source with a multiline pattern
func NewDecoderFromSourceWithPattern(source *sources.ReplaceableSource, multiLinePattern *regexp.Regexp, tailerInfo *status.InfoRegistry) Decoder {
	// Syslog-formatted files get a specialized pipeline that produces
	// StateStructured messages and bypasses multi-line/truncation logic.
	if source.Config().Format == config.SyslogFormat {
		return newSyslogFileDecoder(source, tailerInfo)
	}

	var lineParser parsers.Parser
	framing := framer.UTF8Newline
	switch source.GetSourceType() {
	case sources.KubernetesSourceType:
		lineParser = kubernetes.New()
	case sources.DockerSourceType:
		if pkgconfigsetup.Datadog().GetBool("logs_config.use_podman_logs") {
			// podman's on-disk logs are in kubernetes format
			lineParser = kubernetes.New()
		} else {
			lineParser = dockerfile.New()
		}
	case sources.IntegrationSourceType:
		lineParser = integrations.New()
	default:
		encodingInfo := status.NewMappedInfo("Encoding")
		switch source.Config().Encoding {
		case config.UTF16BE:
			lineParser = encodedtext.New(encodedtext.UTF16BE)
			framing = framer.UTF16BENewline
			encodingInfo.SetMessage("Encoding", "utf-16-be")
		case config.UTF16LE:
			lineParser = encodedtext.New(encodedtext.UTF16LE)
			framing = framer.UTF16LENewline
			encodingInfo.SetMessage("Encoding", "utf-16-le")
		case config.SHIFTJIS:
			lineParser = encodedtext.New(encodedtext.SHIFTJIS)
			framing = framer.SHIFTJISNewline
			encodingInfo.SetMessage("Encoding", "shift-jis")
		default:
			lineParser = noop.New()
			framing = framer.UTF8Newline
			encodingInfo.SetMessage("Encoding", "utf-8")
		}
		tailerInfo.Register(encodingInfo)
	}

	return NewDecoderWithFraming(source, lineParser, framing, multiLinePattern, tailerInfo)
}

// newSyslogFileDecoder builds a decoder for syslog-formatted log files.
//
// It uses UTF-8 newline framing (one syslog message per line) paired with
// a syslog parser that produces StateStructured messages containing all
// syslog metadata. A NoopLineHandler is used instead of SingleLineHandler
// to prevent truncation logic from corrupting the structured messages.
func newSyslogFileDecoder(source *sources.ReplaceableSource, tailerInfo *status.InfoRegistry) Decoder {
	maxMessageSize := source.Config().GetMaxMessageSizeBytes(pkgconfigsetup.Datadog())
	inputChan := make(chan *message.Message)
	outputChan := make(chan *message.Message)
	detectedPattern := &DetectedPattern{}

	lineHandler := NewNoopLineHandler(outputChan)
	lineParser := NewSingleLineParser(lineHandler, syslogparser.NewFileParser())
	f := framer.NewFramer(lineParser.process, framer.UTF8Newline, maxMessageSize)

	encodingInfo := status.NewMappedInfo("Encoding")
	encodingInfo.SetMessage("Encoding", "utf-8")
	tailerInfo.Register(encodingInfo)

	formatInfo := status.NewMappedInfo("Format")
	formatInfo.SetMessage("Format", config.SyslogFormat)
	tailerInfo.Register(formatInfo)

	return New(inputChan, outputChan, f, lineParser, lineHandler, detectedPattern)
}
