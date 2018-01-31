// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2018 Datadog, Inc.

package processor

import (
	"fmt"
	"regexp"

	"strings"

	"github.com/DataDog/datadog-agent/pkg/util"
	log "github.com/cihub/seelog"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/pb"
	"github.com/golang/protobuf/proto"
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
			content, err := p.computeContent(msg, redactedMessage)
			if err == nil {
				msg.SetContent(content)
				p.outputChan <- msg
			} else {
				log.Error("unable to serialize msg", err)
			}
		}
	}
}

func (p *Processor) computeContent(msg message.Message, redactedMessage []byte) ([]byte, error) {

	// TODO Remove occurrences of "severity" (it is now "status")
	// Compute the status
	var status string
	if msg.GetSeverity() != nil {
		status = string(msg.GetSeverity())
	} else {
		status = "info"
	}

	// Compute the hostname
	hostname, err := util.GetHostname()
	if err != nil {
		// this scenario is not likely to happen since the agent can not start without a hostname
		hostname = "unknown"
	}

	// Build the protocol buffer payload
	payload := &pb.LogPayload{
		ApiKey: p.apikey,
		Log: &pb.Log{
			Message:   string(redactedMessage),
			Status:    status,
			Timestamp: msg.GetTimestamp(),
			Hostname:  hostname,
			Service:   msg.GetOrigin().LogSource.Config.Service,
			Source:    msg.GetOrigin().LogSource.Config.Source,
			Category:  msg.GetOrigin().LogSource.Config.SourceCategory,
			Tags:      strings.Split(string(msg.GetTagsPayload()), ","),
		},
	}

	// TODO Remove "logset" from configuration files as it is deprecated
	// Append logset if necessary
	logset := msg.GetOrigin().LogSource.Config.Logset
	if logset != "" {
		payload.Logset = logset
	}

	// Convert the protocol buffer to a flat byte array
	body, err := payload.Marshal()
	if err != nil {
		return nil, err
	}

	// We prepend the body length encoded as a base 128 Varint to the array.
	// (see https://developers.google.com/protocol-buffers/docs/encoding#varints)
	// For example:
	// BEFORE ENCODE (300 bytes)       AFTER ENCODE (302 bytes)
	// +---------------+               +--------+---------------+
	// | Protobuf Data |-------------->| Length | Protobuf Data |
	// |  (300 bytes)  |               | 0xAC02 |  (300 bytes)  |
	// +---------------+               +--------+---------------+
	content := append(proto.EncodeVarint(uint64(len(body))), body...)

	return content, nil
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
