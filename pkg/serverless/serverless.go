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
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/serverless/aws"
	"github.com/DataDog/datadog-agent/pkg/serverless/flush"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	extensionName = "datadog-agent"

	routeRegister      string = "/2020-01-01/extension/register"
	routeEventNext     string = "/2020-01-01/extension/event/next"
	routeInitError     string = "/2020-01-01/extension/init/error"
	routeSubscribeLogs string = "/2020-08-15/logs"

	headerExtName     string = "Lambda-Extension-Name"
	headerExtID       string = "Lambda-Extension-Identifier"
	headerExtErrType  string = "Lambda-Extension-Function-Error-Type"
	headerContentType string = "Content-Type"

	requestTimeout     time.Duration = 5 * time.Second
	clientReadyTimeout time.Duration = 2 * time.Second

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
)

// ID is the extension ID within the AWS Extension environment.
type ID string

// ErrorEnum are errors reported to the AWS Extension environment.
type ErrorEnum string

// String returns the string value for this ID.
func (i ID) String() string {
	return string(i)
}

// String returns the string value for this ErrorEnum.
func (e ErrorEnum) String() string {
	return string(e)
}

// Payload is the payload read in the response while subscribing to
// the AWS Extension env.
type Payload struct {
	EventType          string `json:"eventType"`
	DeadlineMs         int64  `json:"deadlineMs"`
	InvokedFunctionArn string `json:"invokedFunctionArn"`
	ShutdownReason     string `json:"shutdownReason"`
	//    RequestId string `json:"requestId"` // unused
}

// Register registers the serverless daemon and subscribe to INVOKE and SHUTDOWN messages.
// Returns either (the serverless ID assigned by the serverless daemon + the api key as read from
// the environment) or an error.
func Register() (ID, error) {
	var err error

	// create the POST register request
	// we will want to add here every configuration field that the serverless
	// agent supports.

	payload := bytes.NewBuffer(nil)
	payload.Write([]byte(`{"events":["INVOKE", "SHUTDOWN"]}`))

	var request *http.Request
	var response *http.Response

	if request, err = http.NewRequest(http.MethodPost, buildURL(routeRegister), payload); err != nil {
		return "", fmt.Errorf("Register: can't create the POST register request: %v", err)
	}
	request.Header.Set(headerExtName, extensionName)

	// call the service to register and retrieve the given Id
	client := &http.Client{Timeout: 5 * time.Second}
	if response, err = client.Do(request); err != nil {
		return "", fmt.Errorf("Register: error while POST register route: %v", err)
	}

	// read the response
	// -----------------

	var body []byte
	if body, err = ioutil.ReadAll(response.Body); err != nil {
		return "", fmt.Errorf("Register: can't read the body: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return "", fmt.Errorf("Register: didn't receive an HTTP 200: %v -- Response body content: %v", response.StatusCode, string(body))
	}

	// read the ID
	// -----------

	id := response.Header.Get(headerExtID)
	if len(id) == 0 {
		return "", fmt.Errorf("Register: didn't receive an identifier -- Response body content: %v", string(body))
	}

	return ID(id), nil
}

// SubscribeLogs subscribes to the logs collection on the platform.
// We send a request to AWS to subscribe for logs, indicating on which port we
// are opening an HTTP server, to receive logs from AWS.
// When we are receiving logs on this HTTP server, we're pushing them in a channel
// tailed by the Logs Agent pipeline, these logs then go through the regular
// Logs Agent pipeline to finally be sent on the intake when we receive a FLUSH
// call from the Lambda function / client.
// logsType contains the type of logs for which we are subscribing, possible
// value: platform, extension and function.
func SubscribeLogs(id ID, httpAddr string, logsType []string) error {
	var err error
	var request *http.Request
	var response *http.Response
	var jsonBytes []byte

	if _, err := url.ParseRequestURI(httpAddr); err != nil || httpAddr == "" {
		return fmt.Errorf("SubscribeLogs: wrong http addr provided: %s", httpAddr)
	}

	// send a hit on a route to subscribe to the logs collection feature
	// --------------------

	log.Debug("Subscribing to Logs for types:", logsType)

	if jsonBytes, err = json.Marshal(map[string]interface{}{
		"destination": map[string]string{
			"URI":      httpAddr,
			"protocol": "HTTP",
		},
		"types": logsType,
		"buffering": map[string]int{ // TODO(remy): these should be better defined
			"timeoutMs": 1000,
			"maxBytes":  262144,
			"maxItems":  1000,
		},
	}); err != nil {
		return fmt.Errorf("SubscribeLogs: can't marshal subscribe JSON: %s", err)
	}

	if request, err = http.NewRequest(http.MethodPut, buildURL(routeSubscribeLogs), bytes.NewBuffer(jsonBytes)); err != nil {
		return fmt.Errorf("SubscribeLogs: can't create the PUT request: %v", err)
	}
	request.Header.Set(headerExtID, id.String())
	request.Header.Set(headerContentType, "application/json")

	client := &http.Client{
		Transport: &http.Transport{IdleConnTimeout: requestTimeout},
		Timeout:   requestTimeout,
	}
	if response, err = client.Do(request); err != nil {
		return fmt.Errorf("SubscribeLogs: while PUT subscribe request: %s", err)
	}

	if response.StatusCode >= 300 {
		return fmt.Errorf("SubscribeLogs: received an HTTP %s", response.Status)
	}

	return nil
}

// ReportInitError reports an init error to the environment.
func ReportInitError(id ID, errorEnum ErrorEnum) error {
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
func WaitForNextInvocation(stopCh chan struct{}, daemon *Daemon, metricsChan chan []metrics.MetricSample, id ID, coldstart bool) error {
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

	if payload.EventType == "INVOKE" {
		log.Debug("Received invocation event")
		aws.SetARN(payload.InvokedFunctionArn)
		daemon.StartInvocation()
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
	}
	if payload.EventType == "SHUTDOWN" {
		log.Debug("Received shutdown event. Reason: " + payload.ShutdownReason)

		if strings.ToLower(payload.ShutdownReason) == "timeout" {
			metricTags := getTagsForEnhancedMetrics()
			sendTimeoutEnhancedMetric(metricTags, metricsChan)
		}

		daemon.Stop()
		stopCh <- struct{}{}
	}

	return nil
}

func buildURL(route string) string {
	prefix := os.Getenv("AWS_LAMBDA_RUNTIME_API")
	if len(prefix) == 0 {
		return fmt.Sprintf("http://localhost:9001%s", route)
	}
	return fmt.Sprintf("http://%s%s", prefix, route)
}
