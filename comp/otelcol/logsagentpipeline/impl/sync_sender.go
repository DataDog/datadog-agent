// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package logsagentpipelineimpl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"

	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	"github.com/DataDog/datadog-agent/comp/logs-library/processor"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	logsagentpipeline "github.com/DataDog/datadog-agent/comp/otelcol/logsagentpipeline/def"
	logscompression "github.com/DataDog/datadog-agent/comp/serializer/logscompression/def"
	pkgconfigmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/compression"
)

// SyncSender sends log messages to the Datadog logs intake in the calling goroutine and reports the
// outcome of every message, so that the logs agent exporter can hand delivery errors to the
// OpenTelemetry exporterhelper instead of dropping them inside the asynchronous pipeline.
//
// It sends to the reliable and unreliable endpoints.
// ***********************************************************************************************
// WARNING (TODO):
//   - A failure on one of reliable endpoints makes the caller retry the messages on all of them.
//   - Multi-Region Failover endpoints are not supported and are ignored.
//
// Should/must(?) be improved in future.
type SyncSender struct {
	processor      *processor.SyncProcessor
	compressor     compression.Compressor
	reliable       []*http.BlockingDestination
	unreliable     []*http.BlockingDestination
	maxBatchSize   int
	maxContentSize int
	// inFlight bounds the payloads being sent across concurrent calls to Send.
	inFlight chan struct{}
}

// maxConcurrencyPerPipeline mirrors the concurrency of the HTTP sender of each logs pipeline, in
// comp/logs-library/pipeline/provider.go.
const maxConcurrencyPerPipeline = 10

var (
	_ logsagentpipeline.SyncSender        = (*SyncSender)(nil)
	_ logsagentpipeline.SyncSenderFactory = (*Agent)(nil)
)

// NewSyncSender returns a SyncSender configured like the logs agent built from the same deps.
func NewSyncSender(deps Dependencies) (*SyncSender, error) {
	return buildSyncSender(deps.Config, deps.Log, deps.Hostname, deps.Compression, deps.IntakeOrigin)
}

// NewSyncSender returns a SyncSender configured like the agent.
func (a *Agent) NewSyncSender() (logsagentpipeline.SyncSender, error) {
	s, err := buildSyncSender(a.config, a.log, a.hostname, a.compression, a.intakeOrigin)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func buildSyncSender(cfg pkgconfigmodel.Reader, logger log.Component, hostname hostnameinterface.Component, compressionFactory logscompression.Component, intakeOrigin config.IntakeOrigin) (*SyncSender, error) {
	endpoints, err := config.BuildHTTPEndpoints(cfg, intakeTrackType, config.AgentJSONIntakeProtocol, intakeOrigin)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoints: %w", err)
	}
	processingRules, err := config.GlobalProcessingRules(cfg)
	if err != nil {
		return nil, fmt.Errorf("invalid processing rules: %w", err)
	}

	compressor := compressionFactory.NewCompressor(compression.NoneKind, 0)
	if endpoints.Main.UseCompression {
		compressor = compressionFactory.NewCompressor(endpoints.Main.CompressionKind, endpoints.Main.CompressionLevel)
	}
	s := &SyncSender{
		processor:      processor.NewSyncProcessor(processingRules, processor.NewJSONEncoder(cfg.GetBool("logs_config.use_container_timestamp")), hostname),
		compressor:     compressor,
		maxBatchSize:   endpoints.BatchMaxSize,
		maxContentSize: endpoints.BatchMaxContentSize,
		inFlight:       make(chan struct{}, sendConcurrency(cfg, endpoints)),
	}

	newDestination := func(endpoint config.Endpoint) *http.BlockingDestination {
		return http.NewBlockingDestination(endpoint, http.JSONContentType, cfg)
	}

	for _, endpoint := range endpoints.GetReliableEndpoints() {
		if !endpoint.IsMRF {
			s.reliable = append(s.reliable, newDestination(endpoint))
		}
	}
	for _, endpoint := range endpoints.GetUnReliableEndpoints() {
		if !endpoint.IsMRF {
			s.unreliable = append(s.unreliable, newDestination(endpoint))
		}
	}
	if len(s.reliable) == 0 {
		return nil, errors.New("no reliable logs endpoint is configured")
	}
	if len(s.reliable) > 1 {
		logger.Warnf("logs are sent synchronously to %d reliable endpoints: when one of them fails, the logs are retried on all of them, and the endpoints that already accepted them receive duplicates", len(s.reliable))
	}
	return s, nil
}

// sendConcurrency returns the number of payloads that the HTTP sender of the logs pipelines sends at
// the same time.
func sendConcurrency(cfg pkgconfigmodel.Reader, endpoints *config.Endpoints) int {
	perPipeline := maxConcurrencyPerPipeline
	if endpoints.BatchMaxConcurrentSend > 0 {
		perPipeline = endpoints.BatchMaxConcurrentSend
	}
	return max(1, cfg.GetInt("logs_config.pipelines")*perPipeline)
}

// Send processes msgs and sends them to the intake, split into as many payloads as its limits
// require. Payloads are sent concurrently, within a limit shared by concurrent calls. Unreliable
// endpoints are served on a best-effort basis and their errors are not reported.
func (s *SyncSender) Send(ctx context.Context, msgs []*message.Message) []error {
	errs := make([]error, len(msgs))
	fail := func(b payloadBatch, err error) {
		for _, i := range b.members {
			errs[i] = err
		}
	}
	var wg sync.WaitGroup
	for _, b := range s.batch(msgs, errs) {
		select {
		case s.inFlight <- struct{}{}:
		case <-ctx.Done():
			fail(b, ctx.Err())
			continue
		}
		wg.Go(func() {
			defer func() { <-s.inFlight }()
			if err := s.deliver(ctx, b.payload); err != nil {
				fail(b, err)
			}
		})
	}
	wg.Wait()
	return errs
}

// payloadBatch is a payload and the indexes of the messages it carries.
type payloadBatch struct {
	payload *message.Payload
	members []int
}

// batch processes msgs and serializes the ones to send into payloads that respect the intake limits.
// It records in errs the messages that cannot be sent.
func (s *SyncSender) batch(msgs []*message.Message, errs []error) []payloadBatch {
	var batches []payloadBatch
	w := s.newPayloadWriter()
	flush := func() {
		if len(w.members) == 0 {
			w.discard()
			return
		}
		payload, err := w.finish(s.compressor.ContentEncoding())
		if err != nil {
			for _, i := range w.members {
				errs[i] = err
			}
			return
		}
		batches = append(batches, payloadBatch{payload: payload, members: w.members})
	}

	for i, msg := range msgs {
		send, err := s.processor.Process(msg)
		if err != nil {
			errs[i] = err
			continue
		}
		if !send {
			continue
		}
		if size := len(msg.GetContent()); size > s.maxContentSize {
			errs[i] = fmt.Errorf("log message of %d bytes exceeds the payload limit of %d bytes", size, s.maxContentSize)
			continue
		}
		if !w.add(msg, i) {
			flush()
			w = s.newPayloadWriter()
			w.add(msg, i)
		}
	}
	flush()
	return batches
}

// deliver sends payload to every endpoint and returns the errors of the reliable ones.
func (s *SyncSender) deliver(ctx context.Context, payload *message.Payload) error {
	if len(s.reliable) == 1 && len(s.unreliable) == 0 {
		return s.reliable[0].Send(ctx, payload)
	}
	errs := make([]error, len(s.reliable))
	var wg sync.WaitGroup
	for i, d := range s.reliable {
		wg.Go(func() { errs[i] = d.Send(ctx, payload) })
	}
	for _, d := range s.unreliable {
		wg.Go(func() { _ = d.Send(ctx, payload) })
	}
	wg.Wait()
	return errors.Join(errs...)
}

// payloadWriter serializes messages into one compressed payload.
type payloadWriter struct {
	buffer     *sender.MessageBuffer
	serializer sender.Serializer
	encoded    *bytes.Buffer
	stream     compression.StreamCompressor
	written    *countingWriter
	members    []int
	// err is the first serialization error, after which the payload is unusable.
	err error
}

func (s *SyncSender) newPayloadWriter() *payloadWriter {
	encoded := &bytes.Buffer{}
	stream := s.compressor.NewStreamCompressor(encoded)
	return &payloadWriter{
		buffer:     sender.NewMessageBuffer(s.maxBatchSize, s.maxContentSize),
		serializer: sender.NewArraySerializer(),
		encoded:    encoded,
		stream:     stream,
		written:    &countingWriter{w: stream},
	}
}

// add appends the message at index to the payload. It returns false when the payload is full.
func (w *payloadWriter) add(msg *message.Message, index int) bool {
	if !w.buffer.AddMessage(msg) {
		return false
	}
	w.members = append(w.members, index)
	if w.err == nil {
		w.err = w.serializer.Serialize(msg, w.written)
	}
	return true
}

// finish returns the payload, or the error that made it unusable.
func (w *payloadWriter) finish(encoding string) (*message.Payload, error) {
	if w.err == nil {
		w.err = w.serializer.Finish(w.written)
	}
	if err := w.stream.Close(); w.err == nil {
		w.err = err
	}
	if w.err != nil {
		return nil, fmt.Errorf("unable to encode the payload: %w", w.err)
	}
	return message.NewPayload(w.buffer.GetMessages(), w.encoded.Bytes(), encoding, w.written.n), nil
}

// discard releases the resources of a payload that is not sent.
func (w *payloadWriter) discard() {
	_ = w.stream.Close()
}

type countingWriter struct {
	w io.Writer
	n int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += n
	return n, err
}
