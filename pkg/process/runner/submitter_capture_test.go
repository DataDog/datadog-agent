// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	forwarder "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
	"github.com/DataDog/datadog-agent/comp/process/types"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	configmodel "github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	"github.com/DataDog/datadog-agent/pkg/process/util/api"
	"github.com/DataDog/datadog-agent/pkg/process/util/api/headers"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

func newProcessCaptureSubmitter(t *testing.T, name string) (*CheckSubmitter, *telemetrycapture.Manager, telemetrycapture.Control) {
	t.Helper()
	stream := telemetrycapture.Processes
	if name == checks.ConnectionsCheckName {
		stream = telemetrycapture.Connections
	}
	m := telemetrycapture.NewManager("process-agent", "fixture", "fixture")
	t.Cleanup(m.Close)
	require.NoError(t, m.Register(telemetrycapture.Capability{Stream: stream, Cadence: 17 * time.Second}))
	control := telemetrycapture.Control{ProtocolVersion: telemetrycapture.ProtocolVersion, SessionID: "process-capture-session"}
	_, err := m.Prepare(telemetrycapture.PrepareRequest{Control: control, Streams: []telemetrycapture.Stream{stream}})
	require.NoError(t, err)
	_, err = m.Activate(control)
	require.NoError(t, err)
	s := &CheckSubmitter{
		CaptureManager: m, captureCadences: &sync.Map{}, log: logmock.New(t),
		hostname: "fixture-host", exit: make(chan struct{}),
		resultsQueue: map[string]*api.WeightedQueue{name: api.NewWeightedQueue(8, 1<<20)},
		submitFuncs:  make(map[string]submitFunc),
	}
	s.SetCaptureCadence(name, 17*time.Second)
	return s, m, control
}

func captureProcessMessages(name string) []model.MessageBody {
	if name == checks.ConnectionsCheckName {
		return []model.MessageBody{
			&model.CollectorConnections{HostName: "fixture-host", GroupId: 41, GroupSize: 2},
			&model.CollectorConnections{HostName: "fixture-host", GroupId: 41, GroupSize: 2},
		}
	}
	return []model.MessageBody{
		&model.CollectorProc{HostName: "fixture-host", GroupId: 41, GroupSize: 2},
		&model.CollectorProc{HostName: "fixture-host", GroupId: 41, GroupSize: 2},
	}
}

func startCaptureConsumer(s *CheckSubmitter, queue *api.WeightedQueue) func() {
	var consumer sync.WaitGroup
	consumer.Add(1)
	go func() { defer consumer.Done(); s.consumePayloads(queue) }()
	return func() { queue.Stop(); consumer.Wait() }
}

func TestCaptureObservesCompleteOrderedGroupsBeforeNormalDelivery(t *testing.T) {
	for _, name := range []string{checks.ProcessCheckName, checks.ConnectionsCheckName} {
		t.Run(name, func(t *testing.T) {
			s, m, control := newProcessCaptureSubmitter(t, name)
			start := time.Now().Truncate(time.Second).Add(123456789 * time.Nanosecond)
			s.Submit(start, name, &types.Payload{Message: captureProcessMessages(name)})
			s.SetCaptureCadence(name, 19*time.Second)
			queue := s.resultsQueue[name]
			item, ok := queue.Poll()
			require.True(t, ok)
			result := item.(*checkResult)
			require.Equal(t, start, result.collectedAt)
			require.Equal(t, 17*time.Second, result.cadence)
			original := make([][]byte, len(result.payloads))
			for i := range result.payloads {
				original[i] = bytes.Clone(result.payloads[i].body)
				result.payloads[i].headers.Set("Authorization", "fixture-credential")
			}
			calls := 0
			s.submitFuncs[name] = func(payload transaction.BytesPayloads, h http.Header) (chan forwarder.Response, error) {
				require.Equal(t, original[calls], payload[0].GetContent())
				require.Equal(t, "fixture-credential", h.Get("Authorization"))
				require.Equal(t, uint64(1), m.Status().FinalSequence, "one complete group must precede its first submission")
				calls++
				if calls == len(original) {
					queue.Stop()
				}
				responses := make(chan forwarder.Response)
				close(responses)
				return responses, nil
			}
			queue.Add(result)
			s.consumePayloads(queue)
			require.Equal(t, 2, calls)
			for i := range result.payloads {
				result.payloads[i].body[0] ^= 0xff
				result.payloads[i].headers.Set(headers.HostHeader, "changed-host")
			}
			batch, err := m.Read(telemetrycapture.ReadRequest{Control: control})
			require.NoError(t, err)
			defer batch.Release()
			require.Len(t, batch.Records, 1)
			record := batch.Records[0]
			require.Equal(t, start, record.CollectedAt)
			require.Equal(t, 17*time.Second, record.Cadence)
			require.Len(t, record.Payload.Chunks, 2)
			for i, chunk := range record.Payload.Chunks {
				require.Equal(t, original[i], chunk.Body)
				require.Equal(t, "fixture-host", chunk.Headers[headers.HostHeader])
				require.NotContains(t, chunk.Headers, "Authorization")
			}
			decoded, err := api.DecodeCaptureGroup(record)
			require.NoError(t, err)
			require.Len(t, decoded, 2)
		})
	}
}

func TestCaptureFailurePreservesTrackedDeliveryAndErrors(t *testing.T) {
	for _, overflow := range []bool{false, true} {
		for _, failDelivery := range []bool{false, true} {
			t.Run(fmt.Sprintf("overflow=%t/failDelivery=%t", overflow, failDelivery), func(t *testing.T) {
				s, m, control := newProcessCaptureSubmitter(t, checks.ProcessCheckName)
				if overflow {
					for range telemetrycapture.MaxRecords {
						r := m.Begin(telemetrycapture.Processes, time.Now(), time.Second, 1)
						require.NotNil(t, r)
						defer r.Discard()
					}
				}
				calls := 0
				deliveryErr := errors.New("fixture forwarder failure")
				s.submitFuncs[checks.ProcessCheckName] = func(payload transaction.BytesPayloads, h http.Header) (chan forwarder.Response, error) {
					calls++
					if h.Get(headers.HostHeader) != "fixture-host" || len(payload[0].GetContent()) == 0 {
						t.Error("capture changed the production submission")
					}
					if failDelivery {
						return nil, deliveryErr
					}
					responses := make(chan forwarder.Response, 2)
					responses <- forwarder.Response{StatusCode: http.StatusOK}
					responses <- forwarder.Response{StatusCode: http.StatusAccepted}
					close(responses)
					return responses, nil
				}
				stop := startCaptureConsumer(s, s.resultsQueue[checks.ProcessCheckName])
				defer stop()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				err := s.SubmitForHost(ctx, time.Now(), checks.ProcessCheckName, "fixture-host", &types.Payload{Message: captureProcessMessages(checks.ProcessCheckName)})
				stop()
				if failDelivery {
					require.ErrorIs(t, err, deliveryErr)
					require.Equal(t, 1, calls)
				} else {
					require.NoError(t, err)
					require.Equal(t, 2, calls)
				}
				if overflow {
					require.Equal(t, telemetrycapture.Failed, m.Status().State)
				} else {
					batch, err := m.Read(telemetrycapture.ReadRequest{Control: control})
					require.NoError(t, err)
					defer batch.Release()
					require.Len(t, batch.Records, 1)
					require.Len(t, batch.Records[0].Payload.Chunks, 2)
				}
			})
		}
	}
}

func TestCaptureExcludesDroppedAndUnrelatedChecks(t *testing.T) {
	s, m, control := newProcessCaptureSubmitter(t, checks.ProcessCheckName)
	s.dropCheckPayloads = []string{checks.ProcessCheckName}
	queue := s.resultsQueue[checks.ProcessCheckName]
	result := s.messagesToCheckResult(time.Now(), checks.ProcessCheckName, captureProcessMessages(checks.ProcessCheckName))
	result.deliveryResult = make(chan error, 1)
	queue.Add(result)
	stop := startCaptureConsumer(s, queue)
	select {
	case err := <-result.deliveryResult:
		require.EqualError(t, err, "required process payloads are disabled")
	case <-time.After(5 * time.Second):
		t.Fatal("dropped tracked group was not acknowledged")
	}
	stop()
	// Unknown and real-time checks never inspect their chunk data.
	s.observeCaptureResult(&checkResult{name: checks.RTProcessCheckName})
	s.observeCaptureResult(&checkResult{name: checks.DiscoveryCheckName})
	batch, err := m.Read(telemetrycapture.ReadRequest{Control: control})
	require.NoError(t, err)
	defer batch.Release()
	require.Empty(t, batch.Records)
	require.Equal(t, telemetrycapture.Active, m.Status().State)
}

type captureCadenceCheck struct{ testCheck }

func (*captureCadenceCheck) SupportsRunOptions() bool { return true }

func TestCaptureCadenceUsesSchedulerValidatedInterval(t *testing.T) {
	for _, seconds := range []int{8, 9} {
		t.Run(fmt.Sprint(seconds), func(t *testing.T) {
			cfg := configmock.New(t)
			cfg.Set("process_config.intervals.process", seconds, configmodel.SourceAgentRuntime)
			cfg.Set("process_config.intervals.process_realtime", 2, configmodel.SourceAgentRuntime)
			s := &CheckSubmitter{captureCadences: &sync.Map{}}
			l, err := NewRunnerWithChecks(cfg, nil, nil, nil, true, nil)
			require.NoError(t, err)
			l.Submitter = s
			_, err = l.runnerForCheck(&captureCadenceCheck{testCheck{name: checks.ProcessCheckName}})
			require.NoError(t, err)
			expected := time.Duration(seconds) * time.Second
			if seconds == 9 {
				expected = checks.ProcessCheckDefaultInterval
			}
			require.Equal(t, expected, s.captureCadence(checks.ProcessCheckName))
		})
	}
}
