// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/daemon"
	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	"github.com/DataDog/datadog-agent/pkg/serverless/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/registration"
	"github.com/DataDog/datadog-agent/pkg/serverless/tags"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	headerExtID      string = "Lambda-Extension-Identifier"
	headerExtErrType string = "Lambda-Extension-Function-Error-Type"

	requestTimeout      time.Duration = 5 * time.Second
	safetyBufferTimeout time.Duration = 20 * time.Millisecond

	// FatalNoAPIKey is the error reported to the AWS Extension environment when
	// no API key has been set. Unused until we can report error
	// without stopping the extension.
	FatalNoAPIKey ErrorEnum = "Fatal.NoAPIKey"
	// FatalDogstatsdInit is the error reported to the AWS Extension environment when
	// DogStatsD fails to initialize properly. Unused until we can report error
	// without stopping the extension.
	FatalDogstatsdInit ErrorEnum = "Fatal.DogstatsdInit"
	// FatalBadEndpoint is the error reported to the AWS Extension environment when
	// bad endpoints have been configured. Unused until we can report error
	// without stopping the extension.
	FatalBadEndpoint ErrorEnum = "Fatal.BadEndpoint"
	// FatalConnectFailed is the error reported to the AWS Extension environment when
	// a connection failed.
	FatalConnectFailed ErrorEnum = "Fatal.ConnectFailed"

	// Invoke event
	Invoke RuntimeEvent = "INVOKE"
	// Shutdown event
	Shutdown RuntimeEvent = "SHUTDOWN"

	// Timeout is one of the possible ShutdownReasons
	Timeout ShutdownReason = "timeout"
)

// ShutdownReason is an AWS Shutdown reason
type ShutdownReason string

// RuntimeEvent is an AWS Runtime event
type RuntimeEvent string

// ErrorEnum are errors reported to the AWS Extension environment.
type ErrorEnum string

// String returns the string value for this ErrorEnum.
func (e ErrorEnum) String() string {
	return string(e)
}

// String returns the string value for this ShutdownReason.
func (s ShutdownReason) String() string {
	return string(s)
}

// InvocationHandler is the invocation handler signature
type InvocationHandler func(doneChannel chan bool, daemon *daemon.Daemon, arn string, requestID string)

// Payload is the payload read in the response while subscribing to
// the AWS Extension env.
type Payload struct {
	EventType          RuntimeEvent   `json:"eventType"`
	DeadlineMs         int64          `json:"deadlineMs"`
	InvokedFunctionArn string         `json:"invokedFunctionArn"`
	ShutdownReason     ShutdownReason `json:"shutdownReason"`
	RequestID          string         `json:"requestId"`
}

// FlushableAgent allows flushing
type FlushableAgent interface {
	Flush()
}

// WaitForNextInvocation makes a blocking HTTP call to receive the next event from AWS.
// Note that for now, we only subscribe to INVOKE and SHUTDOWN events.
// Write into stopCh to stop the main thread of the running program.
func WaitForNextInvocation(stopCh chan struct{}, daemon *daemon.Daemon, id registration.ID) error {
	var err error
	var request *http.Request
	var response *http.Response

	if request, err = http.NewRequest(http.MethodGet, registration.NextUrl(), nil); err != nil {
		return fmt.Errorf("WaitForNextInvocation: can't create the GET request: %v", err)
	}
	request.Header.Set(headerExtID, id.String())

	// make a blocking HTTP call to wait for the next event from AWS
	client := &http.Client{Timeout: 0} // this one should never timeout
	if response, err = client.Do(request); err != nil {
		return fmt.Errorf("WaitForNextInvocation: while GET next route: %v", err)
	}
	// we received an INVOKE or SHUTDOWN event
	daemon.StoreInvocationTime(time.Now())

	var body []byte
	if body, err = io.ReadAll(response.Body); err != nil {
		return fmt.Errorf("WaitForNextInvocation: can't read the body: %v", err)
	}
	defer response.Body.Close()

	var payload Payload
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("WaitForNextInvocation: can't unmarshal the payload: %v", err)
	}

	if payload.EventType == Invoke {
		functionArn := removeQualifierFromArn(payload.InvokedFunctionArn)
		callInvocationHandler(daemon, functionArn, payload.DeadlineMs, safetyBufferTimeout, payload.RequestID, handleInvocation)
	} else if payload.EventType == Shutdown {
		// Log collection can be safely called multiple times, so ensure we start log collection during a SHUTDOWN event too in case an INVOKE event is never received
		daemon.StartLogCollection()
		log.Debug("Received shutdown event. Reason: " + payload.ShutdownReason)
		isTimeout := strings.ToLower(payload.ShutdownReason.String()) == Timeout.String()
		if isTimeout {
			coldStartTags := daemon.ExecutionContext.GetColdStartTagsForRequestID(daemon.ExecutionContext.LastRequestID())
			metricTags := tags.AddColdStartTag(daemon.ExtraTags.Tags, coldStartTags.IsColdStart, coldStartTags.IsProactiveInit)
			metricTags = tags.AddInitTypeTag(metricTags)
			metrics.SendTimeoutEnhancedMetric(metricTags, daemon.MetricAgent.Demux)
			metrics.SendErrorsEnhancedMetric(metricTags, time.Now(), daemon.MetricAgent.Demux)
		}
		err := daemon.ExecutionContext.SaveCurrentExecutionContext()
		if err != nil {
			log.Warnf("Unable to save the current state. Failed with: %s", err)
		}
		daemon.Stop()
		stopCh <- struct{}{}
	}

	return nil
}

func callInvocationHandler(daemon *daemon.Daemon, arn string, deadlineMs int64, safetyBufferTimeout time.Duration, requestID string, invocationHandler InvocationHandler) {
	timeout := computeTimeout(time.Now(), deadlineMs, safetyBufferTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	doneChannel := make(chan bool)
	daemon.TellDaemonRuntimeStarted()
	go invocationHandler(doneChannel, daemon, arn, requestID)
	select {
	case <-ctx.Done():
		log.Debug("Timeout detected, finishing the current invocation now to allow receiving the SHUTDOWN event")
		// Tell the Daemon that the runtime is done (even though it isn't, because it's timing out) so that we can receive the SHUTDOWN event
		daemon.TellDaemonRuntimeDone()
		return
	case <-doneChannel:
		return
	}
}

func handleInvocation(doneChannel chan bool, daemon *daemon.Daemon, arn string, requestID string) {
	log.Debug("Received invocation event...")
	daemon.ExecutionContext.SetFromInvocation(arn, requestID)
	daemon.StartLogCollection()
	coldStartTags := daemon.ExecutionContext.GetColdStartTagsForRequestID(requestID)

	if daemon.MetricAgent != nil {
		metricTags := tags.AddColdStartTag(daemon.ExtraTags.Tags, coldStartTags.IsColdStart, coldStartTags.IsProactiveInit)
		metricTags = tags.AddInitTypeTag(metricTags)
		metrics.SendInvocationEnhancedMetric(metricTags, daemon.MetricAgent.Demux)
	} else {
		log.Error("Could not send the invocation enhanced metric")
	}

	if coldStartTags.IsColdStart {
		daemon.UpdateStrategy()
	}

	// immediately check if we should flush data
	if daemon.ShouldFlush(flush.Starting) {
		log.Debugf("The flush strategy %s has decided to flush at moment: %s", daemon.GetFlushStrategy(), flush.Starting)
		daemon.TriggerFlush(false)
	} else {
		log.Debugf("The flush strategy %s has decided to not flush at moment: %s", daemon.GetFlushStrategy(), flush.Starting)
	}

	daemon.WaitForDaemon()
	doneChannel <- true
}

func computeTimeout(now time.Time, deadlineMs int64, safetyBuffer time.Duration) time.Duration {
	currentTimeInMs := now.UnixNano() / int64(time.Millisecond)
	return time.Duration((deadlineMs-currentTimeInMs)*int64(time.Millisecond) - int64(safetyBuffer))
}

func removeQualifierFromArn(functionArn string) string {
	functionArnTokens := strings.Split(functionArn, ":")
	tokenLength := len(functionArnTokens)

	if tokenLength > 7 {
		functionArnTokens = functionArnTokens[:tokenLength-1]
		return strings.Join(functionArnTokens, ":")
	}
	return functionArn
}
