// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"regexp"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/framer"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/dockerfile"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/encodedtext"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// NewDecoderFromSource creates a new decoder from a log source
func NewDecoderFromSource(source *sources.ReplaceableSource) *Decoder {
	return NewDecoderFromSourceWithPattern(source, nil)
}

// NewDecoderFromSourceWithPattern creates a new decoder from a log source with a multiline pattern
func NewDecoderFromSourceWithPattern(source *sources.ReplaceableSource, multiLinePattern *regexp.Regexp) *Decoder {

	// TODO: remove those checks and add to source a reference to a tagProvider and a lineParser.
	var lineParser parsers.Parser
	framing := framer.UTF8Newline
	switch source.GetSourceType() {
	case sources.KubernetesSourceType:
		lineParser = kubernetes.New()
	case sources.DockerSourceType:
		if coreConfig.Datadog.GetBool("logs_config.use_podman_logs") {
			// podman's on-disk logs are in kubernetes format
			lineParser = kubernetes.New()
		} else {
			lineParser = dockerfile.New()
		}
	default:
		switch source.Config().Encoding {
		case config.UTF16BE:
			lineParser = encodedtext.New(encodedtext.UTF16BE)
			framing = framer.UTF16BENewline
		case config.UTF16LE:
			lineParser = encodedtext.New(encodedtext.UTF16LE)
			framing = framer.UTF16LENewline
		case config.SHIFTJIS:
			lineParser = encodedtext.New(encodedtext.SHIFTJIS)
			framing = framer.SHIFTJISNewline
		default:
			lineParser = noop.New()
			framing = framer.UTF8Newline
		}
	}

	return NewDecoderWithFraming(source, lineParser, framing, multiLinePattern)
}
