// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package processor

import (
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// A Processor updates messages from an inputChan and pushes
// in an outputChan.
type Processor struct {
	inputChan  chan message.Message
	outputChan chan message.Message
	encoder    Encoder
}

// New returns an initialized Processor.
func New(inputChan, outputChan chan message.Message, encoder Encoder) *Processor {
	return &Processor{
		inputChan:  inputChan,
		outputChan: outputChan,
		encoder:    encoder,
	}
}

// Start starts the Processor.
func (p *Processor) Start() {
	go p.run()
}

// run starts processing the messages in inputChan.
func (p *Processor) run() {
	for msg := range p.inputChan {
		if shouldProcess, redactedMsg := applyRedactingRules(msg); shouldProcess {
			content, err := p.encoder.encode(msg, redactedMsg)
			if err != nil {
				log.Error("unable to encode msg ", err)
				continue
			}
			msg.SetContent(content)
			p.outputChan <- msg
		}
	}
}

// applyRedactingRules returns given a message if we should process it or not,
// and a copy of the message with some fields redacted, depending on config
func applyRedactingRules(msg message.Message) (bool, []byte) {
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
