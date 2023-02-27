// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package registration

import (
	"bytes"
	"fmt"
	"net/http"
	"time"
)

const (
	extensionName = "datadog-agent"
	headerExtName = "Lambda-Extension-Name"

	//HeaderExtID is the header name for the extension identifier
	HeaderExtID = "Lambda-Extension-Identifier"
)

// RegisterExtension registers the serverless daemon and subscribe to INVOKE and SHUTDOWN messages.
// Returns either (the serverless ID assigned by the serverless daemon + the api key as read from
// the environment) or an error.
func RegisterExtension(runtimeURL string, registrationRoute string, timeout time.Duration) (ID, error) {

	extesionRegistrationURL := BuildURL(runtimeURL, registrationRoute)
	payload := createRegistrationPayload()

	request, err := buildRegisterRequest(headerExtName, extensionName, extesionRegistrationURL, payload)
	if err != nil {
		return "", fmt.Errorf("registerExtension: can't create the POST register request: %v", err)
	}

	response, err := sendRequest(&http.Client{Timeout: timeout}, request)
	if err != nil {
		return "", fmt.Errorf("registerExtension: error while POST register route: %v", err)
	}
	defer response.Body.Close()

	if !isAValidResponse(response) {
		return "", fmt.Errorf("registerExtension: didn't receive an HTTP 200")
	}

	id := extractID(response)
	if len(id) == 0 {
		return "", fmt.Errorf("registerExtension: didn't receive an identifier")
	}

	return ID(id), nil
}

func createRegistrationPayload() *bytes.Buffer {
	payload := bytes.NewBuffer(nil)
	payload.Write([]byte(`{"events":["INVOKE", "SHUTDOWN"]}`))
	return payload
}

func extractID(response *http.Response) string {
	return response.Header.Get(HeaderExtID)
}

func isAValidResponse(response *http.Response) bool {
	return response.StatusCode == 200
}

func buildRegisterRequest(headerExtensionName string, extensionName string, url string, payload *bytes.Buffer) (*http.Request, error) {
	request, err := http.NewRequest(http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}
	request.Header.Set(headerExtensionName, extensionName)
	return request, nil
}

func sendRequest(client HTTPClient, request *http.Request) (*http.Response, error) {
	return client.Do(request)
}
