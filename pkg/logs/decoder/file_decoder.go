// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"regexp"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/parser"
)

// NewDecoderFromSource creates a new decoder from a log source
func NewDecoderFromSource(source *config.LogSource) *Decoder {
	return NewDecoderFromSourceWithPattern(source, nil)
}

// NewDecoderFromSourceWithPattern creates a new decoder from a log source with a multiline pattern
func NewDecoderFromSourceWithPattern(source *config.LogSource, multiLinePattern *regexp.Regexp) *Decoder {

	// TODO: remove those checks and add to source a reference to a tagProvider and a lineParser.
	var lineParser parser.Parser
	var matcher EndLineMatcher
	switch source.GetSourceType() {
	case config.KubernetesSourceType:
		lineParser = parser.KubernetesFormat
		matcher = &NewLineMatcher{}
	case config.DockerSourceType:
		if coreConfig.Datadog.GetBool("logs_config.use_podman_logs") {
			// podman's on-disk logs are in kubernetes format
			lineParser = parser.KubernetesFormat
		} else {
			lineParser = parser.DockerFileFormat
		}
		matcher = &NewLineMatcher{}
	default:
		switch source.Config.Encoding {
		case config.UTF16BE:
			lineParser = parser.NewEncodedText(parser.UTF16BE)
			matcher = NewBytesSequenceMatcher(Utf16beEOL, 2)
		case config.UTF16LE:
			lineParser = parser.NewEncodedText(parser.UTF16LE)
			matcher = NewBytesSequenceMatcher(Utf16leEOL, 2)
		case config.SHIFTJIS:
			lineParser = parser.NewEncodedText(parser.SHIFTJIS)
			// No special handling required for the newline matcher since Shift JIS does not use
			// newline characters (0x0a) as the second byte of a multibyte sequence.
			matcher = &NewLineMatcher{}
		default:
			lineParser = parser.Noop
			matcher = &NewLineMatcher{}
		}
	}

	return NewDecoderWithEndLineMatcher(source, lineParser, matcher, multiLinePattern)
}
