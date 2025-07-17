// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"golang.org/x/time/rate"

	"github.com/DataDog/datadog-agent/pkg/dyninst/actuator"
	"github.com/DataDog/datadog-agent/pkg/dyninst/decode"
	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
	"github.com/DataDog/datadog-agent/pkg/dyninst/output"
	"github.com/DataDog/datadog-agent/pkg/dyninst/symbol"
	"github.com/DataDog/datadog-agent/pkg/dyninst/uploader"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type sink struct {
	controller   *Controller
	decoder      Decoder
	symbolicator symbol.Symbolicator
	programID    ir.ProgramID
	service      string
	logUploader  *uploader.LogsUploader
}

var _ actuator.Sink = &sink{}

// We don't want to be too noisy about decoding errors, but we do want to learn
// about them and we don't want to bail out completely.
var decodingErrorLogLimiter = rate.NewLimiter(rate.Every(1*time.Minute), 10)

func (s *sink) HandleEvent(event output.Event) error {
	var buf bytes.Buffer
	// TODO: Find a way to report a partial failure of a single probe.
	probe, err := s.decoder.Decode(decode.Event{
		Event:       event,
		ServiceName: s.service,
	}, s.symbolicator, &buf)
	if err != nil {
		if decodingErrorLogLimiter.Allow() {
			log.Warnf(
				"failed to decode event in service %s: %v",
				s.service, err,
			)
		} else {
			log.Tracef(
				"failed to decode event in service %s: %v",
				s.service, err,
			)
		}
		// TODO: Report failures to the controller to remove the relevant probe
		// or program.
		return nil
	}
	s.controller.setProbeMaybeEmitting(s.programID, probe)
	s.logUploader.Enqueue(json.RawMessage(buf.Bytes()))
	return nil
}

func (s *sink) Close() {
	if s.logUploader != nil {
		s.logUploader.Close()
	}
	if closer, ok := s.symbolicator.(io.Closer); ok {
		if err := closer.Close(); err != nil {
			log.Warnf("failed to close symbolicator: %v", err)
		}
	}
}

type noopSink struct{}

func (n noopSink) Close()                         {}
func (n noopSink) HandleEvent(output.Event) error { return nil }
