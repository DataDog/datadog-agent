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
	"net/url"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/httpclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

type Handler struct {
	allowIMDSEndpoints bool
	httpTimeout        time.Duration
	httpClientProvider httpclient.Provider
	isURLAllowed       func(string) bool
	encryptionStore    *encryptioncontext.Store
}

func (h *Handler) WithHttpClientProvider(httpClientProvider httpclient.Provider) *Handler {
	h.httpClientProvider = httpClientProvider
	return h
}

// WithEncryptionStore enables secret inputs (encryptedCredentials). Without a
// store, they are rejected.
func (h *Handler) WithEncryptionStore(encryptionStore *encryptioncontext.Store) *Handler {
	h.encryptionStore = encryptionStore
	return h
}

func NewHttpRequestAction(cfg *config.Config) *Handler {
	return &Handler{
		allowIMDSEndpoints: cfg.AllowIMDSEndpoint,
		httpTimeout:        cfg.HTTPTimeout,
		httpClientProvider: httpclient.NewDefaultProvider(cfg),
		isURLAllowed:       cfg.IsURLInAllowlist,
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

	// Raw mode only; see ResponseModeRaw. Unset keeps the default behavior.
	ResponseMode      *string      `json:"responseMode,omitempty"`
	BodyBase64        string       `json:"bodyBase64,omitempty"`
	EncryptResponseTo *ResponseKey `json:"encryptResponseTo,omitempty"`

	// Added by wf-actions-server for ExecuteOboWithSecretInputs, in either mode.
	// The sealed payload is SecretInputs, or RawSecretInputs in raw mode.
	EncryptionContext    encryptioncontext.EncryptionContext `json:"encryptionContext,omitempty"`
	EncryptedCredentials string                              `json:"encryptedCredentials,omitempty"`
}

// SecretInputs are the inputs a caller sends sealed, so they never appear in
// the task's plain inputs.
type SecretInputs struct {
	RequestHeaders []Header `json:"requestHeaders,omitempty"`
}

type RequestOutputs struct {
	Status  int         `json:"status"`
	Body    interface{} `json:"body,omitempty"`
	Headers http.Header `json:"headers"`

	// Raw mode only.
	BodyBase64    string `json:"bodyBase64,omitempty"`
	EncryptedBody string `json:"encryptedBody,omitempty"`
	BodyTruncated bool   `json:"bodyTruncated,omitempty"`
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
	if err := validateResponseMode(inputs); err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, err.Error())
	}
	if isRawMode(inputs) {
		return h.runRaw(ctx, task, inputs, credential)
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
	baseURL := ""
	if credential != nil {
		baseURL = credential.HttpDetails.BaseURL
	}
	if baseURL != "" && !hasSameDomain(inputs.URL, baseURL) {
		e := errors.New("specified URL does not match base URL configured in the Connection")
		return nil, util.DefaultActionErrorWithDisplayError(e, e.Error())
	}
	// The base URL check above, and the redirect policy below, keep secret
	// headers on the connection's host.
	if err := h.addSecretHeaders(inputs, headers, credential); err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, err.Error())
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
		// followRedirect: false must stop before the redirect is followed, not after.
		DisableRedirects: inputs.FollowRedirect != nil && !*inputs.FollowRedirect,
		CheckRedirectURL: h.redirectPolicy(baseURL),
		Transport: &httpclient.RunnerHttpTransportConfig{
			AllowExpiredCertificate: boolValueOrDefault(inputs.AllowExpiredCertificate, false),
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

// addSecretHeaders opens the task's secret inputs, if any, and adds their
// headers. Secret values are not evaluated as templates, and a secret header
// can't replace one set by the request, the connection or the runner.
//
// A user OAuth2 connection takes its Authorization only from the secret
// inputs, so a user's token never sits in the task's plain inputs.
func (h *Handler) addSecretHeaders(inputs RequestInputs, headers map[string]string, credential *privateconnection.PrivateCredentials) error {
	userToken := credential != nil && credential.Type == privateconnection.UserOAuth2TokenAuthType
	if userToken {
		for name := range headers {
			if strings.EqualFold(name, "Authorization") {
				return errors.New("a user OAuth2 connection takes Authorization only from the secret inputs")
			}
		}
	}
	hasContext := inputs.EncryptionContext != (encryptioncontext.EncryptionContext{})
	if inputs.EncryptedCredentials == "" && !hasContext {
		if userToken {
			return errors.New("a user OAuth2 connection needs the user's token as a secret input")
		}
		return nil
	}
	if inputs.EncryptedCredentials == "" || !hasContext {
		return errors.New("secret inputs need both encryptedCredentials and encryptionContext")
	}
	if h.encryptionStore == nil {
		return errors.New("secret inputs are not supported by this runner")
	}
	secrets, err := encryptioncontext.DecryptInto[SecretInputs](h.encryptionStore, inputs.EncryptionContext, inputs.EncryptedCredentials)
	if err != nil {
		return fmt.Errorf("failed to open secret inputs: %w", err)
	}
	if err := validateHeaders(secrets.RequestHeaders); err != nil {
		return err
	}
	hasAuthorization := false
	for _, header := range secrets.RequestHeaders {
		if header.Key == "" || strings.EqualFold(header.Key, HeaderToScrub) {
			return fmt.Errorf("secret header %q is not allowed", header.Key)
		}
		for existing := range headers {
			if strings.EqualFold(existing, header.Key) {
				return fmt.Errorf("secret header %q is also set by the request, the connection or the runner", header.Key)
			}
		}
		headers[header.Key] = strings.Join(header.Value, ",")
		hasAuthorization = hasAuthorization || strings.EqualFold(header.Key, "Authorization")
	}
	if userToken && !hasAuthorization {
		return errors.New("a user OAuth2 connection needs the user's token as a secret Authorization header")
	}
	return nil
}

// redirectPolicy applies the checks made on the first URL to every redirect hop:
// the runner allowlist, and the connection's base URL when it has one.
func (h *Handler) redirectPolicy(baseURL string) func(*url.URL) error {
	return func(next *url.URL) error {
		if h.isURLAllowed != nil && !h.isURLAllowed(next.String()) {
			return errors.New("redirect url is not allowed by runner policy: check your configuration file")
		}
		if baseURL != "" && !hasSameDomain(next.String(), baseURL) {
			return errors.New("redirect url does not match base URL configured in the Connection")
		}
		return nil
	}
}
