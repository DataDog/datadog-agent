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

// EnableTelemetryCollectionArgs is the set of arguments used to call
// EnableTelemetryCollection
type EnableTelemetryCollectionArgs struct {
	ID                  ID
	RegistrationURL     string
	RegistrationTimeout time.Duration
	LogsType            string
	Port                int
	CollectionRoute     string
	Timeout             int
	MaxBytes            int
	MaxItems            int
}

// EnableTelemetryCollection enables telemetry collections via AWS Telemetry API
func EnableTelemetryCollection(args EnableTelemetryCollectionArgs) error {
	callBackURI := buildCallbackURI(args.Port, args.CollectionRoute)
	payload := buildLogRegistrationPayload(callBackURI, args.LogsType,
		args.Timeout, args.MaxBytes, args.MaxItems)
	return subscribeTelemetry(args.ID, args.RegistrationURL, args.RegistrationTimeout, payload)
}

func subscribeTelemetry(id ID, url string, timeout time.Duration, payload json.Marshaler) error {

	jsonBytes, err := payload.MarshalJSON()
	if err != nil {
		return fmt.Errorf("SubscribeTelemetry: can't marshal subscribe JSON %v", err)
	}

	request, err := buildLogRegistrationRequest(url, HeaderExtID, headerContentType, id, jsonBytes)
	if err != nil {
		return fmt.Errorf("SubscribeTelemetry: can't create the PUT request: %v", err)
	}

	response, err := sendLogRegistrationRequest(&http.Client{
		Transport: &http.Transport{IdleConnTimeout: timeout},
		Timeout:   timeout,
	}, request)
	if err != nil {
		return fmt.Errorf("SubscribeTelemetry: while PUT subscribe request: %s", err)
	}

	defer response.Body.Close()
	if !isValidHTTPCode(response.StatusCode) {
		return fmt.Errorf("SubscribeTelemetry: received an HTTP %s", response.Status)
	}

	return nil
}

func buildLogRegistrationPayload(callBackURI string, logsType string, timeoutMs int, maxBytes int, maxItems int) *TelemetrySubscriptionPayload {
	logsTypeArray := getLogTypesToSubscribe(logsType)
	log.Debugf("Subscribing to Telemetry for %v with buffering timeoutMs=%d, maxBytes=%d, maxItems=%d", logsTypeArray, timeoutMs, maxBytes, maxItems)
	destination := &destination{
		URI:      callBackURI,
		Protocol: "HTTP",
	}
	buffering := &buffering{
		TimeoutMs: timeoutMs,
		MaxBytes:  maxBytes,
		MaxItems:  maxItems,
	}
	schemaVersion := "2022-07-01"
	payload := &TelemetrySubscriptionPayload{
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
				log.Warn("While subscribing to telemetry, unknown log type", part)
			}
		}
		return logsType
	}
	return []string{"platform", "function", "extension"}
}
