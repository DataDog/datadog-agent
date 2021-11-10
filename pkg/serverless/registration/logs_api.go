// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package registration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	headerContentType string = "Content-Type"
)

// EnableLogsCollection enables logs collections via AWS Logs API
func EnableLogsCollection(
	id ID,
	registrationURL string,
	registrationTimeout time.Duration,
	logsType string,
	port int,
	collectionRoute string,
	timeout int,
	maxBytes int,
	maxItems int) error {

	callBackURI := buildCallbackURI(port, collectionRoute)
	payload := buildLogRegistrationPayload(callBackURI, logsType, timeout, maxBytes, maxItems)
	return subscribeLogs(id, registrationURL, registrationTimeout, payload)
}

func subscribeLogs(id ID, url string, timeout time.Duration, payload json.Marshaler) error {

	jsonBytes, err := payload.MarshalJSON()
	if err != nil {
		return fmt.Errorf("SubscribeLogs: can't marshal subscribe JSON %v", err)
	}

	request, err := buildLogRegistrationRequest(url, HeaderExtID, headerContentType, id, jsonBytes)
	if err != nil {
		return fmt.Errorf("SubscribeLogs: can't create the PUT request: %v", err)
	}

	response, err := sendLogRegistrationRequest(&http.Client{
		Transport: &http.Transport{IdleConnTimeout: timeout},
		Timeout:   timeout,
	}, request)
	if err != nil {
		return fmt.Errorf("SubscribeLogs: while PUT subscribe request: %s", err)
	}

	if !isValidHTTPCode(response.StatusCode) {
		return fmt.Errorf("SubscribeLogs: received an HTTP %s", response.Status)
	}

	return nil
}

func buildLogRegistrationPayload(callBackURI string, logsType string, timeoutMs int, maxBytes int, maxItems int) *LogSubscriptionPayload {
	logsTypeArray := getLogTypesToSubscribe(logsType)
	log.Debug("Subscribing to Logs for types:", logsTypeArray)
	destination := &destination{
		URI:      callBackURI,
		Protocol: "HTTP",
	}
	buffering := &buffering{
		TimeoutMs: timeoutMs,
		MaxBytes:  maxBytes,
		MaxItems:  maxItems,
	}
	schemaVersion := "2021-03-18"
	payload := &LogSubscriptionPayload{
		Destination:   *destination,
		Types:         logsTypeArray,
		Buffering:     *buffering,
		SchemaVersion: schemaVersion,
	}
	return payload
}

func buildCallbackURI(httpServerPort int, httpLogsCollectionRoute string) string {
	return fmt.Sprintf("http://sandbox:%d%s", httpServerPort, httpLogsCollectionRoute)
}

func buildLogRegistrationRequest(url string, headerExtID string, headerContentType string, id ID, payload []byte) (*http.Request, error) {
	request, err := http.NewRequest(http.MethodPut, url, bytes.NewBuffer(payload))
	if err != nil {
		return nil, err
	}
	request.Header.Set(headerExtID, id.String())
	request.Header.Set(headerContentType, "application/json")
	return request, nil
}

func sendLogRegistrationRequest(client HTTPClient, request *http.Request) (*http.Response, error) {
	return client.Do(request)
}

func isValidHTTPCode(statusCode int) bool {
	return statusCode < 300
}

func getLogTypesToSubscribe(envLogsType string) []string {
	if len(envLogsType) > 0 {
		var logsType []string
		parts := strings.Split(strings.TrimSpace(envLogsType), " ")
		for _, part := range parts {
			part = strings.ToLower(strings.TrimSpace(part))
			switch part {
			case "function", "platform", "extension":
				logsType = append(logsType, part)
			default:
				log.Warn("While subscribing to logs, unknown log type", part)
			}
		}
		return logsType
	}
	return []string{"platform", "function", "extension"}
}
