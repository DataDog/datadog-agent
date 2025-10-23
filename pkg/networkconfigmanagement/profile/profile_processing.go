// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build ncm

package profile

import (
	"fmt"
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
	Type   MetadataType `json:"type" yaml:"type"`
	Regex  string       `json:"regex" yaml:"regex"`
	Regexp *regexp.Regexp
	Format string `json:"format" yaml:"format"`
}

// MetadataType represents enums for "types" of things than can be typically extracted for NCM
type MetadataType string

const (
	// Timestamp represents capturing a timestamp from the pattern specified
	Timestamp MetadataType = "timestamp"
	// ConfigSize represents capturing a number that would correlate with the size of a configuration
	ConfigSize MetadataType = "config_size"
)

// ValidationRule represents patterns that should be expected from valid output from a command
type ValidationRule struct {
	Type    string `json:"type" yaml:"type"`
	Pattern string `json:"pattern" yaml:"pattern"`
	Regexp  *regexp.Regexp
}

// RedactionRule represents rules for patterns that warrant removal to protect sensitive data or irrelevant information
type RedactionRule struct {
	Regex       string `json:"regex" yaml:"regex"`
	Regexp      *regexp.Regexp
	Replacement string `json:"replacement" yaml:"replacement"`
}

// ExtractedMetadata is a means to hold metadata to be emitted as metrics or sent as part of the payload
type ExtractedMetadata struct {
	Timestamp  int64
	ConfigSize int
}

// ProcessCommandOutput is for applying redactions, validating, and extracting metadata from a configuration pulled from a device
func (p *NCMProfile) ProcessCommandOutput(ct CommandType, output []byte) ([]byte, *ExtractedMetadata, error) {
	if err := p.ValidateOutput(ct, output); err != nil {
		return []byte{}, nil, err
	}
	redactedOutput, err := p.applyRedactions(ct, output)
	if err != nil {
		return []byte{}, nil, err
	}
	metadata, err := p.extractMetadata(ct, output)
	if err != nil {
		return []byte{}, nil, err
	}
	return redactedOutput, metadata, nil
}

func (p *NCMProfile) extractMetadata(ct CommandType, output []byte) (*ExtractedMetadata, error) {
	commandInfo, ok := p.Commands[ct]
	if !ok {
		return nil, fmt.Errorf("no metadata found for command type %s in profile %s", ct, p.Name)
	}
	result := &ExtractedMetadata{}
	metadataParsingRules := commandInfo.ProcessingRules.MetadataRules
	// TODO: iron out a better way to organize parsing these (i.e. by particular units, etc.) and funnel into the correct field
	// TODO: should this just return the `NetworkDeviceConfig` pre-filled with the metadata?
	// TODO: send metrics once retrieved by the main functionality (access to metrics sender for the device)
	for _, rule := range metadataParsingRules {
		if rule.Regexp == nil {
			log.Errorf("profile %q does not have a regexp for metadata rule %s", p.Name, rule.Regex)
			continue
		}
		switch rule.Type {
		case Timestamp:
			match := rule.Regexp.FindSubmatch(output)
			if match == nil || len(match) < 2 {
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
			matches := rule.Regexp.FindSubmatch(output)
			sizeIndex := rule.Regexp.SubexpIndex("Size")
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
		}
	}
	return result, nil
}

// ValidateOutput is a function that will confirm if the output from the CLI command is considered "valid" and returned successfully
func (p *NCMProfile) ValidateOutput(ct CommandType, output []byte) error {
	commandInfo, ok := p.Commands[ct]
	if !ok {
		return fmt.Errorf("no metadata found for command type %s in profile %s", ct, p.Name)
	}
	validationRules := commandInfo.ProcessingRules.ValidationRules
	for _, rule := range validationRules {
		if rule.Regexp == nil {
			return fmt.Errorf("profile %q does not have a regexp for validation rule: %s ", p.Name, rule.Pattern)
		}
		if !rule.Regexp.Match(output) {
			return fmt.Errorf("invalid output (due to rule requiring: %s) for command type %s in profile %s", rule.Pattern, ct, p.Name)
		}
	}
	return nil
}

func (p *NCMProfile) applyRedactions(ct CommandType, output []byte) ([]byte, error) {
	commandInfo, ok := p.Commands[ct]
	if !ok {
		return []byte{}, fmt.Errorf("no metadata found for command type %s in profile %s", ct, p.Name)
	}
	scrubbedOutput, err := commandInfo.scrub(output)
	if err != nil {
		return []byte{}, err
	}
	return scrubbedOutput, nil
}

func (c Commands) scrub(output []byte) ([]byte, error) {
	// check if the scrubber exists
	if c.Scrubber != nil {
		return c.Scrubber.ScrubBytes(output)
	}
	if len(c.ProcessingRules.RedactionRules) > 0 {
		log.Warnf("no rules for redacting found for command %s, skipping redaction", c.CommandType)
	}
	return output, nil
}

func (p *NCMProfile) compileProcessingRules() error {
	for _, command := range p.Commands {
		err := command.compileProcessingRules()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Commands) compileProcessingRules() error {
	var err error

	rules := c.ProcessingRules
	for i := range rules.MetadataRules {
		rules.MetadataRules[i].Regexp, err = regexp.Compile(rules.MetadataRules[i].Regex)
		if err != nil {
			return err
		}
	}
	for i := range rules.ValidationRules {
		rules.ValidationRules[i].Regexp, err = regexp.Compile(rules.ValidationRules[i].Pattern)
		if err != nil {
			return err
		}
	}
	if len(rules.RedactionRules) > 0 {
		// initialize scrubber for command
		c.Scrubber = scrubber.New()
	}
	for i := range rules.RedactionRules {
		rules.RedactionRules[i].Regexp, err = regexp.Compile(rules.RedactionRules[i].Regex)
		if err != nil {
			return err
		}
		replacer := scrubber.Replacer{
			Regex: rules.RedactionRules[i].Regexp,
			Repl:  []byte(rules.RedactionRules[i].Replacement),
		}
		c.Scrubber.AddReplacer(scrubber.SingleLine, replacer)
	}

	return nil
}
