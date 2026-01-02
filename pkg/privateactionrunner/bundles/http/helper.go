// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_http

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/tmpl"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

const MaxResponseErrMsgLen = 750

type FormDataEntry struct {
	Data string
	Opts *multipart.FileHeader
}

func validateHeaders(inputHeaders []Header) error {
	unsupportedHeaders := map[string]struct{}{
		"content-length":    {},
		"host":              {},
		"x-forwarded-for":   {},
		"x-forwarded-host":  {},
		"x-forwarded-proto": {},
		"forwarded":         {},
		"sec-datadog":       {},
	}
	for _, inputHeader := range inputHeaders {
		// Check if the header key (in lowercase) is in the unsupported headers set
		if _, found := unsupportedHeaders[strings.ToLower(inputHeader.Key)]; found {
			return errors.New("unsupported header was supplied")
		}
	}
	return nil
}

func buildHeaders(
	inputHeaders []Header,
	credentials *privateconnection.PrivateCredentials,
	secDatadogHeaderValue string,
) (map[string]string, error) {
	headers := make(map[string]string)
	for _, header := range inputHeaders {
		if strings.ToLower(header.Key) == HeaderToScrub {
			continue
		}
		headers[header.Key] = strings.Join(header.Value, ",")
	}

	// Headers configured in the connection override headers in the action.
	if credentials != nil {
		for _, header := range credentials.HttpDetails.Headers {
			headers[header.Name] = header.Value
		}
	}

	headers["User-Agent"] = "Datadog"
	headers[DdSecurityHeader] = secDatadogHeaderValue

	if credentials != nil {
		switch credentials.Type {
		case privateconnection.BasicAuthType:
			username, password, err := credentials.GetUsernamePasswordBasicAuth()
			if err != nil {
				return nil, err
			}
			creds := fmt.Sprintf("%s:%s", username, password)
			// Encode to base64
			encodedCreds := base64.StdEncoding.EncodeToString([]byte(creds))
			headers["Authorization"] = fmt.Sprintf("%s %s", "Basic", encodedCreds)
		}
	}

	// evaluate header variables
	headers, err := evaluateHeaders(headers, credentials)
	if err != nil {
		return nil, errors.New("cannot evaluate variables in header")
	}
	return headers, nil
}

func buildUrl(
	inputUrl string,
	inputUrlParams []UrlParam,
	credentials *privateconnection.PrivateCredentials,
) (*url.URL, error) {
	parsedUrl, err := url.Parse(inputUrl)
	if err != nil {
		return nil, err
	}

	query := parsedUrl.Query()
	for _, param := range inputUrlParams {
		query.Add(param.Key, param.Value)
	}

	if credentials != nil {
		for _, param := range credentials.HttpDetails.UrlParameters {
			evaluated, err := EvaluateVariable(credentials, param.Value)
			if err != nil {
				return nil, err
			}
			query.Add(param.Name, evaluated)
		}
	}

	parsedUrl.RawQuery = query.Encode()
	return parsedUrl, nil
}

func buildBody(
	inputBody interface{},
	inputContentType string,
	credentials *privateconnection.PrivateCredentials,
) (interface{}, error) {
	if credentials == nil {
		return inputBody, nil
	}

	credentialContentType := credentials.HttpDetails.Body.ContentType
	credentialBody := credentials.HttpDetails.Body.Content
	hasCredentialBody := credentialBody != "" &&
		credentialContentType != "multipart/form-data"

	if hasCredentialBody {
		if inputContentType != "" && !strings.HasPrefix(inputContentType, credentialContentType) {
			return nil, fmt.Errorf(
				"mismatched body content types between inputs %s and connection %s",
				inputContentType,
				credentialContentType)
		}
		evalBodyFromConnection, err := EvaluateVariable(credentials, credentialBody)
		if err != nil {
			return nil, err
		}
		return consolidateBody(credentialContentType, inputBody, evalBodyFromConnection)
	}

	return inputBody, nil
}

func consolidateBody(
	credentialContentType string,
	inputBody interface{},
	evaluatedBodyFromConnection string,
) (interface{}, error) {
	if strings.Contains(credentialContentType, "/json") ||
		strings.Contains(credentialContentType, "+json") {
		if inputBody != nil {
			inputBodyMap, ok := inputBody.(map[string]interface{})
			if !ok {
				return evaluatedBodyFromConnection, errors.New("Fail to parse input body. \n")
			}

			var evaluatedBodyMap map[string]interface{}
			err := json.Unmarshal([]byte(evaluatedBodyFromConnection), &evaluatedBodyMap)
			if err != nil {
				return evaluatedBodyFromConnection, errors.New("Fail to parse evaluated body.\n")
			}

			// Merge maps
			for k, v := range evaluatedBodyMap {
				inputBodyMap[k] = v
			}

			return inputBodyMap, nil
		}
		return evaluatedBodyFromConnection, nil
	} else {
		if inputBody != nil {
			inputBodyStr, ok := inputBody.(string)
			if !ok {
				return evaluatedBodyFromConnection, errors.New("Fail to parse input body \n")
			}
			return fmt.Sprintf("%s\r\n%s", evaluatedBodyFromConnection, inputBodyStr), nil
		}
		return evaluatedBodyFromConnection, nil
	}
}

func buildFormData(
	inputFormData []FormDataField,
	credentials *privateconnection.PrivateCredentials,
) (*bytes.Buffer, string, error) {
	formInputs := make(map[string]FormDataEntry)
	for _, data := range inputFormData {
		if data.Type == nil || *data.Type == "string" {
			formInputs[data.Key] = FormDataEntry{Data: data.Value}
		} else if *data.Type == "datauri" {
			trimmed := strings.TrimSpace(data.Value)
			if !strings.HasPrefix(trimmed, DataUriPrefix) {
				return nil, "", errors.New("Expected base64 data URI for form data " + data.Key)
			}

			mimeEnd := strings.Index(trimmed, ";")
			if mimeEnd < 0 {
				return nil, "", errors.New("Did not find mime type in data URI for " + data.Key)
			}

			base64Index := strings.Index(trimmed, DataUriBase64Prefix)
			if base64Index != mimeEnd+1 {
				return nil, "", errors.New("Did not find base64 in data URI for " + data.Key)
			}

			mime := trimmed[len(DataUriPrefix):mimeEnd]
			b64Data := trimmed[base64Index+len(DataUriBase64Prefix):]
			buffer := bytes.NewBufferString(b64Data)
			formInputs[data.Key] = FormDataEntry{
				Data: buffer.String(),
				Opts: &multipart.FileHeader{
					Filename: data.Key + "." + strings.Split(mime, "/")[1],
				},
			}
		} else {
			return nil, "", fmt.Errorf("unknown form data category %s", *data.Type)
		}
	}

	if credentials == nil {
		if len(inputFormData) > 0 {
			body := new(bytes.Buffer)
			writer := multipart.NewWriter(body)
			for key, val := range formInputs {
				err := writer.WriteField(key, val.Data)
				if err != nil {
					return nil, "", err
				}
			}
			err := writer.Close()
			if err != nil {
				return nil, "", err
			}
			return body, writer.FormDataContentType(), nil
		}
		return nil, "", nil
	}

	credentialContentType := credentials.HttpDetails.Body.ContentType
	credentialBody := credentials.HttpDetails.Body.Content

	hasCredentialFormData := credentialBody != "" &&
		credentialContentType == "multipart/form-data"

	if hasCredentialFormData {
		return buildFormDataWithCredentialFormData(credentials, formInputs)
	} else if len(inputFormData) > 0 {
		body := new(bytes.Buffer)
		writer := multipart.NewWriter(body)
		for key, val := range formInputs {
			err := writer.WriteField(key, val.Data)
			if err != nil {
				return nil, "", err
			}
		}
		err := writer.Close()
		if err != nil {
			return nil, "", err
		}
		return body, writer.FormDataContentType(), nil
	}

	return nil, "", nil
}

func buildFormDataWithCredentialFormData(
	credentials *privateconnection.PrivateCredentials,
	formInputs map[string]FormDataEntry,
) (*bytes.Buffer, string, error) {
	fromCreds := make(map[string]FormDataEntry)
	connectionContentString := credentials.HttpDetails.Body.Content
	var connectionContent []FormDataField
	err := json.Unmarshal([]byte(connectionContentString), &connectionContent)
	if err != nil {
		return nil, "", errors.New("error parsing connection form data to JSON")
	}
	for _, formData := range connectionContent {
		evalKey, err := EvaluateVariable(credentials, formData.Key)
		if err != nil {
			return nil, "", fmt.Errorf("error evaluate variable. %+v", err)
		}
		evalValue, err := EvaluateVariable(credentials, formData.Value)
		if err != nil {
			return nil, "", fmt.Errorf("error evaluate variable. %+v", err)
		}
		fromCreds[evalKey] = FormDataEntry{Data: evalValue}
	}

	formData := make(map[string]FormDataEntry)
	for key, val := range formInputs {
		formData[key] = val
	}
	// value from connection overwrites inputs
	for key, val := range fromCreds {
		formData[key] = val
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, val := range formData {
		err := writer.WriteField(key, val.Data)
		if err != nil {
			return nil, "", err
		}
	}
	err = writer.Close()
	if err != nil {
		return nil, "", err
	}
	return body, writer.FormDataContentType(), nil
}

func evaluateHeaders(
	headers map[string]string,
	credential *privateconnection.PrivateCredentials,
) (map[string]string, error) {
	result := make(map[string]string)
	for k, v := range headers {
		renderedValue, err := EvaluateVariable(credential, v)
		if err != nil {
			return result, err
		}
		result[k] = renderedValue
	}
	return result, nil
}

func getValueOrDefault(input *string, value string) string {
	if input != nil {
		return *input
	}
	return value
}

func boolValueOrDefault(input *bool, value bool) bool {
	if input != nil {
		return *input
	}
	return value
}

func shouldThrowForHTTPErrorStatus(responseStatus int, statusErrorRanges []string) error {
	errorOnStatus := false

	for _, status := range statusErrorRanges {
		if strings.Contains(status, "-") {
			codes := strings.Split(status, "-")
			if len(codes) != 2 {
				return fmt.Errorf("error parsing errorOnStatus input: %v", statusErrorRanges)
			}

			startCode, err1 := strconv.Atoi(codes[0])
			endCode, err2 := strconv.Atoi(codes[1])

			if err1 != nil || err2 != nil || startCode > endCode {
				return fmt.Errorf("error parsing errorOnStatus input: %v", statusErrorRanges)
			}

			if responseStatus >= startCode && responseStatus <= endCode {
				errorOnStatus = true
			}
		} else {
			code, err := strconv.Atoi(status)
			if err != nil {
				return fmt.Errorf("error parsing errorOnStatus input: %v", statusErrorRanges)
			}

			if responseStatus == code {
				errorOnStatus = true
			}
		}
	}

	if errorOnStatus {
		return fmt.Errorf("response status code %d is in {%v}", responseStatus, statusErrorRanges)
	}

	return nil
}

func hasSameDomain(source, target string) bool {
	sourceURL, err := url.Parse(source)
	if err != nil {
		return false
	}

	targetURL, err := url.Parse(target)
	if err != nil {
		return false
	}

	return sourceURL.Host == targetURL.Host
}

func initHTTPRequestHeader(headers map[string]string, req *http.Request) {
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
}

func httpErrResponseToResultErr(resp *http.Response, body string) error {
	if len(body) == 0 {
		return fmt.Errorf("%s", resp.Status)
	} else if len(body) > MaxResponseErrMsgLen {
		return fmt.Errorf("%s: %s... [truncated]", resp.Status, body[:MaxResponseErrMsgLen])
	} else {
		return fmt.Errorf("%s: %s", resp.Status, body)
	}
}

// EvaluateVariable renders variable defined in the connections.
func EvaluateVariable(
	credential *privateconnection.PrivateCredentials,
	value string,
) (string, error) {
	contextMap := prepContextMap(credential)
	template, err := tmpl.Parse(value)
	if err != nil {
		return "", err
	}
	rendered, err := template.Render(contextMap)
	if err != nil {
		return "", err
	}
	return rendered, nil
}

func prepContextMap(credential *privateconnection.PrivateCredentials) map[string]interface{} {
	contextMap := make(map[string]interface{})
	if credential != nil {
		contextMap["Credentials"] = credential.Tokens
		contextMap["$credentials"] = credential.Tokens
		for _, t := range credential.Tokens {
			contextMap[t.Name] = t.Value
		}
	}
	return contextMap
}
