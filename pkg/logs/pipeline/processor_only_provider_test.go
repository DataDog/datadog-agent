// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pipeline

import (
	"testing"
	"time"

	"encoding/json"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/logs/processor"
	"github.com/DataDog/datadog-agent/pkg/logs/sources"
)

// TestProcessorOnlyProviderWithEncoder_PassthroughContent is the content
// contract test for the observer pipeline: messages exiting the pipeline have
// GetContent() equal to the rendered log line, NOT a JSON transport envelope.
// This is the fundamental invariant that lets the observer's pattern extractors
// work on raw log text.
func TestProcessorOnlyProviderWithEncoder_PassthroughContent(t *testing.T) {
	cfg := configmock.New(t)
	p := NewProcessorOnlyProviderWithEncoder(
		&diagnostic.NoopMessageReceiver{},
		nil, // no processing rules
		nil, // no hostname component
		cfg,
		processor.PassthroughEncoder,
	)
	p.Start()
	defer p.Stop()

	logLine := []byte("error: connection refused to 10.0.0.1:5432")
	source := sources.NewLogSource("test", nil)
	msg := message.NewMessageWithSource(logLine, message.StatusError, source, 0)

	p.NextPipelineChan() <- msg

	select {
	case out := <-p.GetOutputChan():
		assert.Equal(t, logLine, out.GetContent(),
			"observer pipeline: GetContent() must return the raw log line, not a JSON envelope")
		assert.Equal(t, message.StateEncoded, out.State,
			"message must be in StateEncoded after PassthroughEncoder")
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message from processor pipeline")
	}
}

// TestNewProcessorOnlyProvider_JSONEnvelopeContract is the regression guard for
// the analyzelogs subcommand: the default provider (no encoder override) still
// wraps messages in a JSON envelope that analyzelogs can unmarshal.
func TestNewProcessorOnlyProvider_JSONEnvelopeContract(t *testing.T) {
	cfg := configmock.New(t)
	p := NewProcessorOnlyProvider(
		&diagnostic.NoopMessageReceiver{},
		nil,
		nil,
		cfg,
	)
	p.Start()
	defer p.Stop()

	logLine := []byte("something happened")
	source := sources.NewLogSource("test", nil)
	msg := message.NewMessageWithSource(logLine, message.StatusInfo, source, 0)

	p.NextPipelineChan() <- msg

	type jsonPayload struct {
		Message string `json:"message"`
		Status  string `json:"status"`
	}

	select {
	case out := <-p.GetOutputChan():
		var payload jsonPayload
		err := json.Unmarshal(out.GetContent(), &payload)
		require.NoError(t, err, "default provider must produce valid JSON (analyzelogs contract)")
		assert.Equal(t, string(logLine), payload.Message,
			"JSON payload message field must match the original log line")
		assert.Equal(t, message.StatusInfo, payload.Status)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for message from processor pipeline")
	}
}
