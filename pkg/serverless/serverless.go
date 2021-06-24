// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package serverless

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/aws"
	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	"github.com/DataDog/datadog-agent/pkg/serverless/registration"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	routeEventNext string = "/2020-01-01/extension/event/next"
	routeInitError string = "/2020-01-01/extension/init/error"

	headerExtID      string = "Lambda-Extension-Identifier"
	headerExtErrType string = "Lambda-Extension-Function-Error-Type"

	requestTimeout     time.Duration = 5 * time.Second
	clientReadyTimeout time.Duration = 2 * time.Second

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
type InvocationHandler func(doneChannel chan bool, daemon *Daemon, arn string, coldstart bool)

// Payload is the payload read in the response while subscribing to
// the AWS Extension env.
type Payload struct {
	EventType          RuntimeEvent   `json:"eventType"`
	DeadlineMs         int64          `json:"deadlineMs"`
	InvokedFunctionArn string         `json:"invokedFunctionArn"`
	ShutdownReason     ShutdownReason `json:"shutdownReason"`
	//    RequestId string `json:"requestId"` // unused
}

// ReportInitError reports an init error to the environment.
func ReportInitError(id registration.ID, errorEnum ErrorEnum) error {
	var err error
	var content []byte
	var request *http.Request
	var response *http.Response

	if content, err = json.Marshal(map[string]string{
		"error": string(errorEnum),
	}); err != nil {
		return fmt.Errorf("ReportInitError: can't write the payload: %s", err)
	}

	if request, err = http.NewRequest(http.MethodPost, buildURL(routeInitError), bytes.NewBuffer(content)); err != nil {
		return fmt.Errorf("ReportInitError: can't create the POST request: %s", err)
	}

	request.Header.Set(headerExtID, id.String())
	request.Header.Set(headerExtErrType, FatalConnectFailed.String())

	client := &http.Client{
		Transport: &http.Transport{IdleConnTimeout: requestTimeout},
		Timeout:   requestTimeout,
	}

	if response, err = client.Do(request); err != nil {
		return fmt.Errorf("ReportInitError: while POST init error route: %s", err)
	}

	if response.StatusCode >= 300 {
		return fmt.Errorf("ReportInitError: received an HTTP %s", response.Status)
	}

	return nil
}

// WaitForNextInvocation makes a blocking HTTP call to receive the next event from AWS.
// Note that for now, we only subscribe to INVOKE and SHUTDOWN events.
// Write into stopCh to stop the main thread of the running program.
func WaitForNextInvocation(stopCh chan struct{}, daemon *Daemon, metricsChan chan []metrics.MetricSample, id registration.ID, coldstart bool) error {
	var err error
	var request *http.Request
	var response *http.Response

	if request, err = http.NewRequest(http.MethodGet, buildURL(routeEventNext), nil); err != nil {
		return fmt.Errorf("WaitForNextInvocation: can't create the GET request: %v", err)
	}
	request.Header.Set(headerExtID, id.String())

	// make a blocking HTTP call to wait for the next event from AWS
	log.Debug("Waiting for next invocation...")
	client := &http.Client{Timeout: 0} // this one should never timeout
	if response, err = client.Do(request); err != nil {
		return fmt.Errorf("WaitForNextInvocation: while GET next route: %v", err)
	}

	daemon.finishInvocationOnce = sync.Once{}

	// we received an INVOKE or SHUTDOWN event
	daemon.StoreInvocationTime(time.Now())

	var body []byte
	if body, err = ioutil.ReadAll(response.Body); err != nil {
		return fmt.Errorf("WaitForNextInvocation: can't read the body: %v", err)
	}
	defer response.Body.Close()

	var payload Payload
	if err := json.Unmarshal(body, &payload); err != nil {
		return fmt.Errorf("WaitForNextInvocation: can't unmarshal the payload: %v", err)
	}

	if payload.EventType == Invoke {
		callInvocationHandler(daemon, payload.InvokedFunctionArn, payload.DeadlineMs, safetyBufferTimeout, coldstart, handleInvocation)
	}
	if payload.EventType == Shutdown {
		log.Debug("Received shutdown event. Reason: " + payload.ShutdownReason)
		isTimeout := strings.ToLower(payload.ShutdownReason.String()) == Timeout.String()
		if isTimeout {
			metricTags := addColdStartTag(daemon.extraTags)
			sendTimeoutEnhancedMetric(metricTags, metricsChan)
		}
		daemon.Stop(isTimeout)
		stopCh <- struct{}{}
	}

	return nil
}

func callInvocationHandler(daemon *Daemon, arn string, deadlineMs int64, safetyBufferTimeout time.Duration, coldstart bool, invocationHandler InvocationHandler) {
	timeout := computeTimeout(time.Now(), deadlineMs, safetyBufferTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	doneChannel := make(chan bool)
	go invocationHandler(doneChannel, daemon, arn, coldstart)
	select {
	case <-ctx.Done():
		log.Debug("Timeout detected, finishing the current invocation now to allow receiving the SHUTDOWN event")
		daemon.FinishInvocation()
		return
	case <-doneChannel:
		return
	}
}

func handleInvocation(doneChannel chan bool, daemon *Daemon, arn string, coldstart bool) {
	daemon.StartInvocation()
	log.Debug("Received invocation event...")
	daemon.ComputeGlobalTags(arn, config.GetConfiguredTags(true))
	aws.SetARN(arn)
	if coldstart {
		ready := daemon.WaitUntilClientReady(clientReadyTimeout)
		if ready {
			log.Debug("Client library registered with extension")
		} else {
			log.Debug("Timed out waiting for client library to register with extension.")
		}
		daemon.UpdateStrategy()
	}

	// immediately check if we should flush data
	// note that since we're flushing synchronously here, there is a scenario
	// where this could be blocking the function if the flush is slow (if the
	// extension is not quickly going back to listen on the "wait next event"
	// route). That's why we use a context.Context with a timeout `flushTimeout``
	// to avoid blocking for too long.
	// This flushTimeout is re-using the forwarder_timeout value.
	if daemon.flushStrategy.ShouldFlush(flush.Starting, time.Now()) {
		log.Debugf("The flush strategy %s has decided to flush the data in the moment: %s", daemon.flushStrategy, flush.Starting)
		flushTimeout := config.Datadog.GetDuration("forwarder_timeout") * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
		daemon.TriggerFlush(ctx, false)
		cancel() // free the resource of the context
	} else {
		log.Debugf("The flush strategy %s has decided to not flush in the moment: %s", daemon.flushStrategy, flush.Starting)
	}
	daemon.WaitForDaemon()
	doneChannel <- true
}

func buildURL(route string) string {
	prefix := os.Getenv("AWS_LAMBDA_RUNTIME_API")
	if len(prefix) == 0 {
		return fmt.Sprintf("http://localhost:9001%s", route)
	}
	return fmt.Sprintf("http://%s%s", prefix, route)
}

func computeTimeout(now time.Time, deadlineMs int64, safetyBuffer time.Duration) time.Duration {
	currentTimeInMs := now.UnixNano() / int64(time.Millisecond)
	return time.Duration((deadlineMs-currentTimeInMs)*int64(time.Millisecond) - int64(safetyBuffer))
}
