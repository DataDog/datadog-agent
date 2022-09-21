// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package processor

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"strconv"
	"sync"

	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/logs/config"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/internal/metrics"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// A Processor updates messages from an inputChan and pushes
// in an outputChan.
type Processor struct {
	inputChan                 chan *message.Message
	outputChan                chan *message.Message
	processingRules           []*config.ProcessingRule
	encoder                   Encoder
	done                      chan struct{}
	diagnosticMessageReceiver diagnostic.MessageReceiver
	traceIDRule               bool
	mu                        sync.Mutex
}

// New returns an initialized Processor.
func New(inputChan, outputChan chan *message.Message, processingRules []*config.ProcessingRule, encoder Encoder, diagnosticMessageReceiver diagnostic.MessageReceiver) *Processor {
	return &Processor{
		inputChan:                 inputChan,
		outputChan:                outputChan,
		processingRules:           processingRules,
		encoder:                   encoder,
		done:                      make(chan struct{}),
		traceIDRule:               coreconfig.Datadog.IsSet(coreconfig.OTLPSection),
		diagnosticMessageReceiver: diagnosticMessageReceiver,
	}
}

// Start starts the Processor.
func (p *Processor) Start() {
	go p.run()
}

// Stop stops the Processor,
// this call blocks until inputChan is flushed
func (p *Processor) Stop() {
	close(p.inputChan)
	<-p.done
}

// Flush processes synchronously the messages that this processor has to process.
func (p *Processor) Flush(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if len(p.inputChan) == 0 {
				return
			}
			msg := <-p.inputChan
			p.processMessage(msg)
		}
	}
}

// run starts the processing of the inputChan
func (p *Processor) run() {
	defer func() {
		p.done <- struct{}{}
	}()
	for msg := range p.inputChan {
		p.processMessage(msg)
		p.mu.Lock() // block here if we're trying to flush synchronously
		//nolint:staticcheck
		p.mu.Unlock()
	}
}

func (p *Processor) processMessage(msg *message.Message) {
	metrics.LogsDecoded.Add(1)
	metrics.TlmLogsDecoded.Inc()
	if shouldProcess, redactedMsg := p.applyRedactingRules(msg); shouldProcess {
		metrics.LogsProcessed.Add(1)
		metrics.TlmLogsProcessed.Inc()

		p.diagnosticMessageReceiver.HandleMessage(*msg, redactedMsg)

		// Encode the message to its final format
		content, err := p.encoder.Encode(msg, redactedMsg)
		if err != nil {
			log.Error("unable to encode msg ", err)
			return
		}
		msg.Content = content
		p.outputChan <- msg
	}
}

// applyRedactingRules returns given a message if we should process it or not,
// and a copy of the message with some fields redacted, depending on config
func (p *Processor) applyRedactingRules(msg *message.Message) (bool, []byte) {
	content := msg.Content
	rules := append(p.processingRules, msg.Origin.LogSource.Config.ProcessingRules...)
	for _, rule := range rules {
		switch rule.Type {
		case config.ExcludeAtMatch:
			if rule.Regex.Match(content) {
				return false, nil
			}
		case config.IncludeAtMatch:
			if !rule.Regex.Match(content) {
				return false, nil
			}
		case config.MaskSequences:
			content = rule.Regex.ReplaceAll(content, rule.Placeholder)
		}
	}
	if p.traceIDRule {
		// OTLP Ingest is in use, so we may be receiving logs with
		// 128-bit trace_id fields embedded. Convert these to 64-bit
		// valid Datadog IDs.
		content = replaceTraceID(content)
	}
	return true, content
}

var traceIDPrefix = []byte(" trace_id=")

func replaceTraceID(content []byte) []byte {
	n := bytes.Index(content, traceIDPrefix)
	if n < 0 {
		// does not contain the trace_id field
		return content
	}
	if len(content)-n-len(traceIDPrefix) < 32 {
		// we need at least 32 characters more for an OpenTelemetry
		// 128-bit trace ID to fit
		return content
	}
	idstart := n + len(traceIDPrefix) // index of ID start
	var i int
	// check that the ID is a valid 16 byte hex number
	for i = idstart; i < len(content); i++ {
		ch := content[i]
		if ch == ' ' {
			// we've reached the end
			break
		}
		if (ch < 'a' || ch > 'f') && (ch < '0' || ch > '9') {
			// invalid character for a hex number
			return content
		}
	}
	if i-n-len(traceIDPrefix) != 32 {
		// the hex ID needs to be exactly 32 characters long
		return content
	}
	// convert the hex to a byte array and take the lower 8 bytes
	// this matches the conversion algorithm of APM
	hexid := content[idstart:i]
	id := make([]byte, 16)
	_, err := hex.Decode(id, hexid)
	if err != nil {
		// can not convert hex number
		return content
	}
	newid := binary.BigEndian.Uint64(id[len(id)-8:])
	newidstr := []byte(strconv.FormatUint(newid, 10))
	// Replace the old ID with the new ID.
	//
	// We can use the same byte slice when replacing because the uint64
	// string is always going to be shorter than 32 bytes because
	// math.MaxUint64 is 20 bytes long as a string
	x := copy(content[idstart:], newidstr)
	x += copy(content[idstart+x:], content[idstart+32:])
	return content[:idstart+x]
}
