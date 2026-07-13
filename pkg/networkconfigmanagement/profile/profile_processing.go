// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package profile

import (
	"bytes"
	"regexp"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/scrubber"
)

// ProcessingRules represent different rules for "processing" including extracting, validating, redacting information from cmd line output
type ProcessingRules struct {
	MetadataRules   []MetadataRule   `json:"metadata" yaml:"metadata"`
	ValidationRules []ValidationRule `json:"validation" yaml:"validation"`
	RedactionRules  []RedactionRule  `json:"redaction" yaml:"redaction"`
}

// MetadataRule represents the rules for parsing metadata from a network device's command
type MetadataRule struct {
	Type   MetadataType   `json:"type" yaml:"type"`
	Regex  *regexp.Regexp `json:"regex" yaml:"regex"`
	Format string         `json:"format" yaml:"format"`
}

// MetadataType represents enums for "types" of things than can be typically extracted for NCM
type MetadataType string

const (
	// Timestamp represents capturing a timestamp from the pattern specified
	Timestamp MetadataType = "timestamp"
	// ConfigSize represents capturing a number that would correlate with the size of a configuration
	ConfigSize MetadataType = "config_size"
	// Author represents the username/identifier of the person who made the latest change if available
	Author MetadataType = "author"
)

// ValidationRule represents patterns that should be expected from valid output from a command
type ValidationRule struct {
	Type    string         `json:"type" yaml:"type"`
	Pattern *regexp.Regexp `json:"pattern" yaml:"pattern"`
}

// RedactionRule represents rules for patterns that warrant removal to protect sensitive data or irrelevant information
type RedactionRule struct {
	Regex       *regexp.Regexp `json:"regex" yaml:"regex"`
	Replacement string         `json:"replacement" yaml:"replacement"`
	Multiline   bool           `json:"multiline" yaml:"multiline"`
}

// ExtractedMetadata is a means to hold metadata to be emitted as metrics or sent as part of the payload
type ExtractedMetadata struct {
	Timestamp  int64
	ConfigSize int
	Author     string
}

// ProcessedConfig holds the results of a profile processing a config.
type ProcessedConfig struct {
	// Raw is the raw config, after preprocessing to remove console noise.
	Raw []byte
	// Redacted is the config after running various redaction rules to hide
	// sensitive data (passwords, etc.)
	Redacted []byte
	// Metadata is the metadata extracted from the config.
	Metadata *ExtractedMetadata
}

// ExtractMetadata extracts available metadata from the given config output
func (p *NCMProfile) ExtractMetadata(config []byte) (*ExtractedMetadata, error) {
	result := &ExtractedMetadata{}
	// TODO: iron out a better way to organize parsing these (i.e. by particular units, etc.) and funnel into the correct field
	// Possibly instead of []MetadataRule there should be a structured object of {timestamp:Rule, size:Rule, etc.}?
	// TODO: should this just return the `NetworkDeviceConfig` pre-filled with the metadata?
	// TODO: send metrics once retrieved by the main functionality (access to metrics sender for the device)
	for _, rule := range p.MetadataRules {
		switch rule.Type {
		case Timestamp:
			match := rule.Regex.FindSubmatch(config)
			if len(match) < 2 {
				log.Warnf("could not parse timestamp for profile %s", p.Name)
				continue
			}
			timestampString := string(match[1])
			timestamp, err := time.Parse(rule.Format, timestampString)
			if err != nil {
				log.Warnf("could not parse timestamp for profile %s", p.Name)
				continue
			}
			result.Timestamp = timestamp.Unix()
		case ConfigSize:
			matches := rule.Regex.FindSubmatch(config)
			sizeIndex := rule.Regex.SubexpIndex("Size")
			if sizeIndex == -1 || matches == nil {
				log.Warnf("could not parse config size for profile %s", p.Name)
				continue
			}
			size, err := strconv.Atoi(string(matches[sizeIndex]))
			if err != nil {
				log.Warnf("could not parse config size for profile %s", p.Name)
				continue
			}
			result.ConfigSize = size
		case Author:
			matches := rule.Regex.FindSubmatch(config)
			if len(matches) < 2 {
				log.Warnf("could not parse author for profile %s", p.Name)
				continue
			}
			author := string(matches[1])
			result.Author = author
		}
	}
	return result, nil
}

func Redact(config []byte, rules []RedactionRule) ([]byte, error) {
	config = normalizeOutput(config)
	if len(rules) == 0 {
		return config, nil
	}
	scrub := scrubber.New()
	for _, rule := range rules {
		replacer := scrubber.Replacer{
			Regex: rule.Regex,
			Repl:  []byte(rule.Replacement),
		}
		mode := scrubber.SingleLine
		if rule.Multiline {
			mode = scrubber.MultiLine
		}
		scrub.AddReplacer(mode, replacer)
	}
	return scrub.ScrubBytes(config)
}

// preprocessConfig applies preprocessing rules to remove extraneous CLI noise.
func (p *NCMProfile) preprocessConfig(config []byte) ([]byte, error) {
	// Apply preproc rules
	config, err := Redact(config, p.Preprocessing)
	if err != nil {
		return []byte{}, err
	}
	// strip all leading/trailing whitespace.
	config = bytes.Trim(config, " \n\t")
	return config, nil
}

// ProcessConfig is for applying redactions and extracting metadata from a configuration pulled from a device
func (p *NCMProfile) ProcessConfig(config []byte) (*ProcessedConfig, error) {
	normalizedOutput := normalizeOutput(config)
	preprocessedOutput, err := p.preprocessConfig(normalizedOutput)
	if err != nil {
		return nil, err
	}
	redactedOutput, err := Redact(preprocessedOutput, p.Redactions)
	if err != nil {
		return nil, err
	}
	// extract metadata from the un-preprocessed output because preprocessing
	// usually removes lines like "!Time: ..."
	metadata, err := p.ExtractMetadata(normalizedOutput)
	if err != nil {
		return nil, err
	}
	return &ProcessedConfig{
		Raw:      preprocessedOutput,
		Redacted: redactedOutput,
		Metadata: metadata,
	}, nil
}

func normalizeOutput(output []byte) []byte {
	return bytes.ReplaceAll(output, []byte{'\r', '\n'}, []byte{'\n'})
}
