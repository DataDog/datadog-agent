// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package processor

import (
	"fmt"
	"regexp"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

var rfc5424Pattern, _ = regexp.Compile("<[0-9]{1,3}>[0-9] ")

// A Processor updates messages from an inputChan and pushes
// in an outputChan
type Processor struct {
	inputChan  chan message.Message
	outputChan chan message.Message
	apiKey     []byte
}

// New returns an initialized Processor
func New(inputChan, outputChan chan message.Message, apiKey, logset string) *Processor {
	if logset != "" {
		apiKey = fmt.Sprintf("%s/%s", apiKey, logset)
	}
	return &Processor{
		inputChan:  inputChan,
		outputChan: outputChan,
		apiKey:     []byte(apiKey),
	}
}

// Start starts the Processor
func (p *Processor) Start() {
	go p.run()
}

// run starts the processing of the inputChan
func (p *Processor) run() {
	for msg := range p.inputChan {
		shouldProcess, redactedMessage := p.applyRedactingRules(msg)
		if shouldProcess {
			apiKey := p.apiKey
			extraContent := p.computeExtraContent(msg)
			payload := p.buildPayload(apiKey, redactedMessage, extraContent)
			msg.SetContent(payload)
			p.outputChan <- msg
		}
	}
}

// computeExtraContent returns additional content to add to a log line.
// For instance, we want to add the timestamp, hostname and a log level
// to messages coming from a file
func (p *Processor) computeExtraContent(msg message.Message) []byte {
	// if the first char is '<', we can assume it's already formatted as RFC5424, thus skip this step
	// (for instance, using tcp forwarding. We don't want to override the hostname & co)
	if len(msg.Content()) > 0 && !p.isRFC5424Formatted(msg.Content()) {
		// fit RFC5424
		// <%pri%>%protocol-version% %timestamp:::date-rfc3339% %HOSTNAME% %$!new-appname% - - - %msg%\n
		extraContent := []byte("")

		// Severity
		if msg.GetSeverity() != nil {
			extraContent = append(extraContent, msg.GetSeverity()...)
		} else {
			extraContent = append(extraContent, config.SevInfo...)
		}

		// Protocol version
		extraContent = append(extraContent, '0')
		extraContent = append(extraContent, ' ')

		// Timestamp
		if msg.GetTimestamp() != "" {
			extraContent = append(extraContent, []byte(msg.GetTimestamp())...)
		} else {
			timestamp := time.Now().UTC().Format(config.DateFormat)
			extraContent = append(extraContent, []byte(timestamp)...)
		}
		extraContent = append(extraContent, ' ')

		// Hostname
		hostname, err := util.GetHostname()
		if err != nil {
			// this scenario is not likely to happen since the agent can not start without a hostname
			hostname = "unknown"
		}
		extraContent = append(extraContent, []byte(hostname)...)
		extraContent = append(extraContent, ' ')

		// Service
		service := msg.GetOrigin().LogSource.Config.Service
		if service != "" {
			extraContent = append(extraContent, []byte(service)...)
		} else {
			extraContent = append(extraContent, '-')
		}

		// Extra
		extraContent = append(extraContent, []byte(" - - ")...)

		// Tags
		extraContent = append(extraContent, msg.GetTagsPayload()...)
		extraContent = append(extraContent, ' ')

		return extraContent
	}
	return nil
}

func (p *Processor) isRFC5424Formatted(content []byte) bool {
	// RFC2424 formatted messages start with `<%pri%>%protocol-version% `
	// pri is 1 to 3 digits, protocol-version is one digit (won't realisticly
	// be more before we kill this custom code)
	// As a result, the start is between 5 and 7 chars.
	if len(content) < 8 { // even is start could be only 5 chars, RFC5424 must have other chars like `-`
		return false
	}
	return rfc5424Pattern.Match(content[:8])
}

// buildPayload returns a processed payload from a raw message
func (p *Processor) buildPayload(apiKey, redactedMessage, extraContent []byte) []byte {
	payload := append(apiKey, ' ')
	if extraContent != nil {
		payload = append(payload, extraContent...)
	}
	payload = append(payload, redactedMessage...)
	payload = append(payload, '\n')
	return payload
}

// applyRedactingRules returns given a message if we should process it or not,
// and a copy of the message with some fields redacted, depending on config
func (p *Processor) applyRedactingRules(msg message.Message) (bool, []byte) {
	content := msg.Content()
	for _, rule := range msg.GetOrigin().LogSource.Config.ProcessingRules {
		switch rule.Type {
		case config.ExcludeAtMatch:
			if rule.Reg.Match(content) {
				return false, nil
			}
		case config.IncludeAtMatch:
			if !rule.Reg.Match(content) {
				return false, nil
			}
		case config.MaskSequences:
			content = rule.Reg.ReplaceAllLiteral(content, rule.ReplacePlaceholderBytes)
		}
	}
	return true, content
}
