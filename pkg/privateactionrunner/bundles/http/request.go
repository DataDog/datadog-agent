// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package com_datadoghq_http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/httpclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

type Handler struct {
	allowIMDSEndpoints bool
	httpTimeout        time.Duration
	httpClientProvider httpclient.Provider
}

func (h *Handler) WithHttpClientProvider(httpClientProvider httpclient.Provider) *Handler {
	h.httpClientProvider = httpClientProvider
	return h
}

func NewHttpRequestAction(cfg *config.Config) *Handler {
	return &Handler{
		allowIMDSEndpoints: cfg.AllowIMDSEndpoint,
		httpTimeout:        cfg.HTTPTimeout,
		httpClientProvider: httpclient.NewDefaultProvider(cfg),
	}
}

type Header struct {
	Key   string   `json:"key"`
	Value []string `json:"value"`
}

type UrlParam struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type FormDataField struct {
	Key   string  `json:"key"`
	Value string  `json:"value"`
	Type  *string `json:"type,omitempty"` // type can be either 'datauri' or 'string'
}

const (
	DataUriPrefix       = "data:"
	DataUriBase64Prefix = "base64,"
	HeaderToScrub       = "runtime-worker-key"
	DdSecurityHeader    = "Sec-Datadog"
)

type RequestInputs struct {
	Verb                    string          `json:"verb"` // Allowed values: HEAD, GET, PUT, POST, DELETE, PATCH
	URL                     string          `json:"url"`  // Must be a valid URL
	Body                    interface{}     `json:"body,omitempty"`
	RequestHeaders          []Header        `json:"requestHeaders,omitempty"`
	URLParams               []UrlParam      `json:"urlParams,omitempty"`
	FormData                []FormDataField `json:"formData,omitempty"`
	FollowRedirect          *bool           `json:"followRedirect,omitempty"`
	AllowExpiredCertificate *bool           `json:"allowExpiredCertificate,omitempty"`
	ErrorOnStatus           []string        `json:"errorOnStatus,omitempty"`
	ResponseParsing         *string         `json:"responseParsing,omitempty"`
	ResponseEncoding        *string         `json:"responseEncoding,omitempty"`
}

type RequestOutputs struct {
	Status  int         `json:"status"`
	Body    interface{} `json:"body,omitempty"`
	Headers http.Header `json:"headers"`
}

func (h *Handler) Run(
	ctx context.Context,
	task *types.Task,
	credential *privateconnection.PrivateCredentials,
) (interface{}, error) {
	inputs, err := types.ExtractInputs[RequestInputs](task)
	if err != nil {
		return nil, util.DefaultActionError(err)
	}

	if err := validateHeaders(inputs.RequestHeaders); err != nil {
		return &RequestOutputs{}, util.DefaultActionErrorWithDisplayError(err, "header is in valid.")
	}
	// HEADERS
	headers, err := buildHeaders(
		inputs.RequestHeaders,
		credential,
		task.Data.Attributes.SecDatadogHeaderValue,
	)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "fail to build header")
	}

	// Validate inputs
	if credential != nil && credential.HttpDetails.BaseURL != "" {
		baseURL := credential.HttpDetails.BaseURL
		if baseURL != "" && !hasSameDomain(inputs.URL, baseURL) {
			e := errors.New("specified URL does not match base URL configured in the Connection")
			return nil, util.DefaultActionErrorWithDisplayError(e, e.Error())
		}
	}

	// BUILD URL
	requestURL, err := buildUrl(inputs.URL, inputs.URLParams, credential)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "fail to build request url")
	}

	// BUILD BODY
	body, err := buildBody(inputs.Body, headers["Content-Type"], credential)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "fail to build request body")
	}
	// BUILD FORM DATA
	formData, formContentType, err := buildFormData(inputs.FormData, credential)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "fail to build request form data")
	}
	// Only one of form data or a request body can be sent in a request
	if formData != nil && body != nil && body != "" {
		e := errors.New("both body and form-data were specified. Please select only one for your request")
		return nil, util.DefaultActionErrorWithDisplayError(e, e.Error())
	}

	// BUILD REQUEST
	var req *http.Request
	if body != nil && body != "" {
		reqBody, err := json.Marshal(body)
		if err != nil {
			return nil, util.DefaultActionErrorWithDisplayError(err, "error encoding JSON request body")
		}
		req, err = http.NewRequestWithContext(ctx, inputs.Verb, requestURL.String(), bytes.NewBuffer(reqBody))
		if err != nil {
			return nil, util.DefaultActionErrorWithDisplayError(err, "error creating request")
		}
		initHTTPRequestHeader(headers, req)
	} else if formData != nil {
		req, err = http.NewRequestWithContext(ctx, inputs.Verb, requestURL.String(), formData)
		if err != nil {
			return nil, util.DefaultActionErrorWithDisplayError(err, "error creating request")
		}
		initHTTPRequestHeader(headers, req)
		req.Header.Set("Content-Type", formContentType)
	} else {
		req, err = http.NewRequestWithContext(ctx, inputs.Verb, requestURL.String(), nil)
		if err != nil {
			return nil, util.DefaultActionErrorWithDisplayError(err, "error creating request")
		}
		initHTTPRequestHeader(headers, req)
	}
	// MAKE REQUEST
	client, err := h.httpClientProvider.NewClient(&httpclient.RunnerHttpClientConfig{
		AllowIMDSEndpoints: h.allowIMDSEndpoints,
		HTTPTimeout:        h.httpTimeout,
		Transport: &httpclient.RunnerHttpTransportConfig{
			InsecureSkipVerify: boolValueOrDefault(inputs.AllowExpiredCertificate, false),
		},
	})
	if err != nil {
		return nil, util.DefaultActionError(err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "failed to make request")
	}
	defer func() { _ = resp.Body.Close() }()

	// CHECK FOR ERROR
	if len(inputs.ErrorOnStatus) > 0 {
		err := shouldThrowForHTTPErrorStatus(resp.StatusCode, inputs.ErrorOnStatus)
		if err != nil {
			return nil, util.DefaultActionError(err)
		}
	} else {
		// Check if FollowRedirect is disabled and response status code indicates a redirection
		if inputs.FollowRedirect != nil &&
			!*inputs.FollowRedirect &&
			resp.StatusCode >= 300 &&
			resp.StatusCode < 400 {
			return nil, util.DefaultActionError(
				fmt.Errorf("server responded with status %d. Enable 'Follow redirect' to let the HTTP action follow redirections", resp.StatusCode),
			)
		}
	}
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, util.DefaultActionError(errors.New("failed to read response body"))
	}
	parsedBody, err := util.ParseResponseBody(
		resp.Header.Get("Content-Type"),
		respBody,
		getValueOrDefault(inputs.ResponseParsing, ""),
		getValueOrDefault(inputs.ResponseEncoding, ""),
		resp.StatusCode,
	)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("failed to convert response body: %w", err))
	}

	// Check if response status code is an error
	if resp.StatusCode >= 400 &&
		resp.StatusCode <= 999 {
		return nil, util.DefaultActionError(httpErrResponseToResultErr(resp, string(respBody)))
	}

	// BUILD AND RETURN OUTPUT
	output := &RequestOutputs{
		Status:  resp.StatusCode,
		Body:    parsedBody,
		Headers: resp.Header,
	}
	return output, nil
}
