// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017 Datadog, Inc.

package processor

import (
	"fmt"
	"time"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// A Processor updates messages from an inputChan and pushes
// in an outputChan
type Processor struct {
	inputChan    chan message.Message
	outputChan   chan message.Message
	apikey       string
	logset       string
	apikeyString []byte
}

// New returns an initialized Processor
func New(inputChan, outputChan chan message.Message, apikey, logset string) *Processor {
	var apikeyString string
	if logset != "" {
		apikeyString = fmt.Sprintf("%s/%s", apikey, logset)
	} else {
		apikeyString = fmt.Sprintf("%s", apikey)
	}
	return &Processor{
		inputChan:    inputChan,
		outputChan:   outputChan,
		apikey:       apikey,
		logset:       logset,
		apikeyString: []byte(apikeyString),
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
			extraContent := p.computeExtraContent(msg)
			apikeyString := p.computeAPIKeyString(msg)
			payload := p.buildPayload(apikeyString, redactedMessage, extraContent)
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
	if len(msg.Content()) > 0 && msg.Content()[0] != '<' {
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
		extraContent = append(extraContent, []byte(config.LogsAgent.GetString("hostname"))...)
		extraContent = append(extraContent, ' ')

		// Service
		service := msg.GetOrigin().LogSource.Service
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

func (p *Processor) computeAPIKeyString(msg message.Message) []byte {
	sourceLogset := msg.GetOrigin().LogSource.Logset
	if sourceLogset != "" {
		return []byte(fmt.Sprintf("%s/%s", p.apikey, sourceLogset))
	}
	return p.apikeyString
}

// buildPayload returns a processed payload from a raw message
func (p *Processor) buildPayload(apikeyString, redactedMessage, extraContent []byte) []byte {
	payload := append(apikeyString, ' ')
	if extraContent != nil {
		payload = append(payload, extraContent...)
	}
	payload = append(payload, redactedMessage...)
	payload = append(payload, '\n') // TODO: move this in decoder
	return payload
}

// applyRedactingRules returns given a message if we should process it or not,
// and a copy of the message with some fields redacted, depending on config
func (p *Processor) applyRedactingRules(msg message.Message) (bool, []byte) {
	content := msg.Content()
	for _, rule := range msg.GetOrigin().LogSource.ProcessingRules {
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
