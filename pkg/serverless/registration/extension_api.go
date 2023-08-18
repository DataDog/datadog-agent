// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package registration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/DataDog/datadog-agent/pkg/serverless/trigger"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	extensionName    = "datadog-agent"
	extensionFeature = "accountId"
	headerExtName    = "Lambda-Extension-Name"
	headerExtFeature = "Lambda-Extension-Accept-Feature"

	//HeaderExtID is the header name for the extension identifier
	HeaderExtID = "Lambda-Extension-Identifier"

	routeEventNext string = "/2020-01-01/extension/event/next"
)

// RegisterExtension registers the serverless daemon and subscribe to INVOKE and SHUTDOWN messages.
// Returns either (the serverless ID assigned by the serverless daemon + the api key as read from
// the environment) or an error.
func RegisterExtension(runtimeURL string, registrationRoute string, timeout time.Duration) (ID, FunctionARN, error) {
	extesionRegistrationURL := BuildURL(registrationRoute)
	payload := createRegistrationPayload()

	request, err := buildRegisterRequest(extesionRegistrationURL, payload)
	if err != nil {
		return "", "", fmt.Errorf("registerExtension: can't create the POST register request: %v", err)
	}

	response, err := sendRequest(&http.Client{Timeout: timeout}, request)
	if err != nil {
		return "", "", fmt.Errorf("registerExtension: error while POST register route: %v", err)
	}
	functionArn, err := extractFunctionARN(response)
	if err != nil {
		return "", "", fmt.Errorf("registerExtension: didn't receive an account ID")
	}
	response.Body.Close()
	if !isAValidResponse(response) {
		return "", "", fmt.Errorf("registerExtension: didn't receive an HTTP 200")
	}

	id := extractID(response)
	if len(id) == 0 {
		return "", "", fmt.Errorf("registerExtension: didn't receive an identifier")
	}

	return ID(id), FunctionARN(functionArn), nil
}

func createRegistrationPayload() *bytes.Buffer {
	payload := bytes.NewBuffer(nil)
	payload.Write([]byte(`{"events":["INVOKE", "SHUTDOWN"]}`))
	return payload
}

func extractID(response *http.Response) string {
	return response.Header.Get(HeaderExtID)
}

func extractFunctionARN(response *http.Response) (string, error) {
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return "", err
	}
	respBody := make(map[string]string)
	err = json.Unmarshal(body, &respBody)
	if err != nil {
		return "", err
	}
	functionName := respBody["functionName"]
	accountId := respBody["accountId"]
	region := os.Getenv("AWS_REGION")
	partition := trigger.GetAWSPartitionByRegion(region)
	return fmt.Sprintf("arn:%s:lambda:%s:%s:function:%s", partition, region, accountId, functionName), nil
}

func isAValidResponse(response *http.Response) bool {
	return response.StatusCode == 200
}

func buildRegisterRequest(url string, payload *bytes.Buffer) (*http.Request, error) {
	request, err := http.NewRequest(http.MethodPost, url, payload)
	if err != nil {
		return nil, err
	}
	request.Header.Set(headerExtName, extensionName)
	request.Header.Set(headerExtFeature, extensionFeature)
	return request, nil
}

func sendRequest(client HTTPClient, request *http.Request) (*http.Response, error) {
	return client.Do(request)
}

// NoOpProcessEvent conforms to the Lambda Runtime API but act as a no-op
// this is required NOT to fail the extension (and customer code) when no api key has been set
func NoOpProcessEvent(ctx context.Context, id ID) error {
	var err error
	var request *http.Request
	var response *http.Response
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
			if request, err = http.NewRequest(http.MethodGet, NextUrl(), nil); err != nil {
				return fmt.Errorf("NoOp WaitForNextInvocation: can't create the GET request: %v", err)
			}
			request.Header.Set(HeaderExtID, id.String())
			// make a blocking HTTP call to wait for the next event from AWS
			client := &http.Client{Timeout: 0} // this one should never timeout
			if response, err = client.Do(request); err != nil {
				return fmt.Errorf("WaitForNextInvocation: while GET next route: %v", err)
			}

			defer response.Body.Close()
			log.Warn("The extension is running as a no-op extension")
		}
	}
}

// NextUrl returns the /next endpoint
func NextUrl() string {
	return BuildURL(routeEventNext)
}
