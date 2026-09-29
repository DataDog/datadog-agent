// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/comp/process/types"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
)

// SubmitForHost delivers every encoded chunk for a supplied host through the
// existing weighted queue, headers and forwarders. The production Submitter
// interface is unchanged. The concrete caller must Start the submitter first.
func (s *CheckSubmitter) SubmitForHost(ctx context.Context, start time.Time, name, hostname string, payload *types.Payload) error {
	if hostname == "" || payload == nil || len(payload.Message) == 0 {
		return errors.New("tracked submission requires a host and nonempty payload")
	}
	if name != checks.ProcessCheckName && name != checks.ConnectionsCheckName {
		return errors.New("tracked submission supports only process and connection payloads")
	}
	if s.shouldDropPayload(name) {
		return fmt.Errorf("required %s payloads are disabled", name)
	}
	messages := payload.Message
	if s.capture != nil {
		var err error
		messages, err = s.capture(name, messages)
		if err != nil {
			return err
		}
	}
	for _, body := range messages {
		switch m := body.(type) {
		case *model.CollectorProc:
			if m == nil || name != checks.ProcessCheckName || m.HostName != hostname {
				return errors.New("process body does not match the requested device")
			}
		case *model.CollectorConnections:
			if m == nil || name != checks.ConnectionsCheckName || m.HostName != hostname {
				return errors.New("connection body does not match the requested device")
			}
		default:
			return errors.New("unsupported tracked message type")
		}
	}
	// The local encoder view shares queues but owns the hostname and request-ID
	// cache. Concurrent devices never mutate the production submitter identity.
	encoder := *s
	encoder.hostname = hostname
	encoder.requestIDCachedHash = nil
	result := encoder.messagesToCheckResult(start, name, messages)
	if result == nil || len(result.payloads) != len(messages) {
		return errors.New("failed to encode all required process chunks")
	}
	result.deliveryContext = ctx
	result.deliveryResult = make(chan error, 1)
	queue := s.resultsQueue[name]
	if queue == nil {
		return errors.New("required process queue is unavailable")
	}
	if err := queue.AddBlocking(ctx, result); err != nil {
		return err
	}
	select {
	case err := <-result.deliveryResult:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-s.exit:
		return errors.New("submitter stopped before delivery completed")
	}
}

func (s *CheckSubmitter) deliverTracked(result *checkResult) error {
	submit, ok := s.submitFuncs[result.name]
	if !ok {
		return fmt.Errorf("no forwarder for %s", result.name)
	}
	for _, payload := range result.payloads {
		if err := result.deliveryContext.Err(); err != nil {
			return err
		}
		responses, err := submit(transaction.NewBytesPayloadsWithoutMetaData([]*[]byte{&payload.body}), payload.headers)
		if err != nil {
			return err
		}
		if responses == nil {
			return errors.New("forwarder returned no delivery responses")
		}
		// DefaultForwarder allocates one buffered response slot per transaction.
		expected := max(1, cap(responses))
		received := 0
		for received < expected {
			select {
			case response, open := <-responses:
				if !open {
					return fmt.Errorf("only %d of %d required delivery responses arrived", received, expected)
				}
				if response.Err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
					return fmt.Errorf("%s delivery failed with HTTP %d", result.name, response.StatusCode)
				}
				received++
			case <-result.deliveryContext.Done():
				return result.deliveryContext.Err()
			case <-s.exit:
				return errors.New("submitter stopped during tracked delivery")
			}
		}
	}
	return nil
}
