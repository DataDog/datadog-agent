// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package decoder

import (
	"regexp"

	coreConfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/decoder/breaker"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/dockerfile"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/encodedtext"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/kubernetes"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/parsers/noop"
)

// NewDecoderFromSource creates a new decoder from a log source
func NewDecoderFromSource(source *config.LogSource) *Decoder {
	return NewDecoderFromSourceWithPattern(source, nil)
}

// NewDecoderFromSourceWithPattern creates a new decoder from a log source with a multiline pattern
func NewDecoderFromSourceWithPattern(source *config.LogSource, multiLinePattern *regexp.Regexp) *Decoder {

	// TODO: remove those checks and add to source a reference to a tagProvider and a lineParser.
	var lineParser parsers.Parser
	var matcher breaker.EndLineMatcher
	switch source.GetSourceType() {
	case config.KubernetesSourceType:
		lineParser = kubernetes.New()
		matcher = &breaker.NewLineMatcher{}
	case config.DockerSourceType:
		switch source.Config.ContainerRuntime {
		case coreConfig.Podman:
			// podman's on-disk logs are in kubernetes format
			lineParser = kubernetes.New()
		default: // default to Docker
			lineParser = dockerfile.New()
		}
		matcher = &breaker.NewLineMatcher{}
	default:
		switch source.Config.Encoding {
		case config.UTF16BE:
			lineParser = encodedtext.New(encodedtext.UTF16BE)
			matcher = breaker.NewBytesSequenceMatcher(breaker.Utf16beEOL, 2)
		case config.UTF16LE:
			lineParser = encodedtext.New(encodedtext.UTF16LE)
			matcher = breaker.NewBytesSequenceMatcher(breaker.Utf16leEOL, 2)
		case config.SHIFTJIS:
			lineParser = encodedtext.New(encodedtext.SHIFTJIS)
			// No special handling required for the newline matcher since Shift JIS does not use
			// newline characters (0x0a) as the second byte of a multibyte sequence.
			matcher = &breaker.NewLineMatcher{}
		default:
			lineParser = noop.New()
			matcher = &breaker.NewLineMatcher{}
		}
	}

	return NewDecoderWithEndLineMatcher(source, lineParser, matcher, multiLinePattern)
}
