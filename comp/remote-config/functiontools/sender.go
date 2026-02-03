// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package functiontools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"

	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
)

// CallbackSender handles sending function tool call results to a callback URL
type CallbackSender struct {
	apiKey      string
	appKey      string
	callbackURL string
}

// CallbackPayload represents the payload sent to the callback URL
type CallbackPayload struct {
	Data CallbackPayloadData `json:"data"`
}

// CallbackPayloadData represents the data field in the callback payload
type CallbackPayloadData struct {
	Type       string                    `json:"type"`
	Attributes CallbackPayloadAttributes `json:"attributes"`
}

// CallbackPayloadAttributes represents the attributes of the callback payload
type CallbackPayloadAttributes struct {
	Data        string `json:"data"`
	Path        string `json:"path"`
	ContentType string `json:"content_type"`
}

// CallbackResult represents the actual result data sent in the payload
type CallbackResult struct {
	CallID           string `json:"call_id"`
	FunctionToolName string `json:"function_tool_name"`
	Output           any    `json:"output"`
	Error            string `json:"error,omitempty"`
}

// NewCallbackSender creates a new CallbackSender with the provided credentials and callback URL
func NewCallbackSender(apiKey, appKey, callbackURL string) *CallbackSender {
	return &CallbackSender{
		apiKey:      apiKey,
		appKey:      appKey,
		callbackURL: callbackURL,
	}
}

// Send sends the function tool call result to the callback URL
func (s *CallbackSender) Send(callID, functionToolName string, output any, err error) error {
	// Build the result object
	result := CallbackResult{
		CallID:           callID,
		FunctionToolName: functionToolName,
		Output:           output,
	}
	if err != nil {
		result.Error = err.Error()
	}

	// Serialize the result to JSON string for the data field
	resultJSON, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal result: %w", marshalErr)
	}

	payload := CallbackPayload{
		Data: CallbackPayloadData{
			Type: "ingest",
			Attributes: CallbackPayloadAttributes{
				Data:        string(resultJSON),
				Path:        "/tmp/output.log",
				ContentType: "application/json",
			},
		},
	}

	jsonData, marshalErr := json.Marshal(payload)
	if marshalErr != nil {
		return fmt.Errorf("failed to marshal payload: %w", marshalErr)
	}

	req, reqErr := http.NewRequest(http.MethodPost, s.callbackURL, bytes.NewBuffer(jsonData))
	if reqErr != nil {
		return fmt.Errorf("failed to create request: %w", reqErr)
	}
	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("DD-API-KEY", s.apiKey)
	req.Header.Set("DD-APPLICATION-KEY", s.appKey)

	// Extract sandbox ID from callback URL and set the request hash header
	// URL format: https://app.datadoghq.com/api/v2/dod-sandboxer/sandbox/{sandbox_id}/ingest
	sandboxIDRegex := regexp.MustCompile(`/sandbox/([^/]+)/ingest`)
	if matches := sandboxIDRegex.FindStringSubmatch(s.callbackURL); len(matches) > 1 {
		pkglog.Errorf("REQUEST HASH SET")
		req.Header.Set("dd-request-hash", matches[1])
	} else {
		panic("no request hash set")
	}

	// Log full request details for debugging
	pkglog.Errorf("[FA DEBUG] HTTP Request:")
	pkglog.Errorf("[FA DEBUG]   Method: %s", req.Method)
	pkglog.Errorf("[FA DEBUG]   URL: %s", req.URL.String())
	pkglog.Errorf("[FA DEBUG]   Headers:")
	for key, values := range req.Header {
		for _, value := range values {
			// Mask sensitive headers
			if key == "Dd-Api-Key" || key == "Dd-Application-Key" {
				if len(value) > 10 {
					pkglog.Errorf("[FA DEBUG]     %s: %s...%s", key, value[:5], value[len(value)-5:])
				} else if len(value) == 0 {
					pkglog.Errorf("[FA DEBUG]     %s: (empty)", key)
				} else {
					pkglog.Errorf("[FA DEBUG]     %s: ***", key)
				}
			} else {
				pkglog.Errorf("[FA DEBUG]     %s: %s", key, value)
			}
		}
	}
	pkglog.Errorf("[FA DEBUG]   Body: %s", string(jsonData))

	client := &http.Client{}
	resp, sendErr := client.Do(req)
	if sendErr != nil {
		return fmt.Errorf("failed to send request: %w", sendErr)
	}
	defer resp.Body.Close()

	// Log response details
	pkglog.Errorf("[FA DEBUG] HTTP Response:")
	pkglog.Errorf("[FA DEBUG]   Status: %d %s", resp.StatusCode, resp.Status)
	respBody, _ := io.ReadAll(resp.Body)
	pkglog.Errorf("[FA DEBUG]   Body: %s", string(respBody))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(respBody))
	}

	pkglog.Infof("Successfully sent result to callback URL (status: %d)", resp.StatusCode)
	return nil
}
