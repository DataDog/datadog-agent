// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_http

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/hpke"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/bundle-support/httpclient"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/encryptioncontext"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
)

// ResponseModeRaw makes request act as a plain HTTP client through the runner,
// for callers such as OAuth discovery, client registration, token exchange and
// refresh, and MCP JSON-RPC. In raw mode, request sends the body bytes verbatim,
// never renders templates, never follows redirects, and returns the status,
// headers and body of every response, including 3xx and 4xx.
//
// The runner's URL allowlist bounds where it may send. The connection's headers
// and credentials apply only to the connection's base URL host, so a secret meant
// for the MCP server never reaches another host such as an identity provider.
//
// Secrets that must not sit in plaintext in the task (an OAuth code, a refresh
// token, a client secret, a user's access token) arrive through
// ExecuteOboWithSecretInputs as encryptedCredentials, sealed to a one-time key
// from prepareEncryption.
const ResponseModeRaw = "raw"

// MaxRawResponseBodyBytes caps the raw response body. Base64 makes it about
// 10.7 MiB in the task result, below the ~15 MB point where agents fail.
const MaxRawResponseBodyBytes = 8 * 1024 * 1024

// RawResponseKeyType is the only key type encryptResponseTo accepts.
const RawResponseKeyType = "hpke-p256-hkdf-sha256-aes256gcm"

// rawResponseHPKEInfo separates response sealing from the per-task input
// sealing in libs/encryptioncontext. The opening side must use the same bytes.
var rawResponseHPKEInfo = []byte("dd-par-http-request-raw-response")

// Hop-by-hop and transport headers are owned by the runner's own HTTP client.
var rawStrippedRequestHeaders = map[string]bool{
	"Connection":          true,
	"Content-Length":      true,
	"Host":                true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Proxy-Connection":    true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
	"Cookie":              true,
	textproto.CanonicalMIMEHeaderKey(HeaderToScrub):    true,
	textproto.CanonicalMIMEHeaderKey(DdSecurityHeader): true,
}

// Cookies set by a private server stay on the runner.
var rawStrippedResponseHeaders = map[string]bool{
	"Set-Cookie": true,
}

// RawSecretInputs is the decrypted encryptedCredentials payload.
// Its headers are added to the caller's, and its body replaces the caller's.
type RawSecretInputs struct {
	Headers    map[string][]string `json:"headers,omitempty"`
	BodyBase64 string              `json:"bodyBase64,omitempty"`
}

// ResponseKey is a Datadog-held public key the response body is sealed to, so
// OAuth token and client registration responses are never stored in plaintext.
type ResponseKey struct {
	KeyType   string `json:"keyType"`
	PublicKey string `json:"publicKey"`
}

func isRawMode(inputs RequestInputs) bool {
	return inputs.ResponseMode != nil && *inputs.ResponseMode == ResponseModeRaw
}

// validateResponseMode keeps the default mode exactly as it was: raw-only inputs
// are refused there, and inputs whose meaning raw mode drops are refused in raw mode.
func validateResponseMode(inputs RequestInputs) error {
	if inputs.ResponseMode != nil && *inputs.ResponseMode != "" && *inputs.ResponseMode != ResponseModeRaw {
		return fmt.Errorf("unsupported responseMode %q", *inputs.ResponseMode)
	}
	if !isRawMode(inputs) {
		if inputs.BodyBase64 != "" || inputs.EncryptResponseTo != nil {
			return errors.New("bodyBase64 and encryptResponseTo require responseMode raw")
		}
		return nil
	}
	switch {
	case inputs.Body != nil && inputs.Body != "":
		return errors.New("responseMode raw takes bodyBase64, not body")
	case len(inputs.FormData) > 0:
		return errors.New("responseMode raw takes bodyBase64, not formData")
	case len(inputs.URLParams) > 0:
		return errors.New("responseMode raw sends the url as given and takes no urlParams")
	case inputs.FollowRedirect != nil && *inputs.FollowRedirect:
		return errors.New("responseMode raw never follows redirects")
	case len(inputs.ErrorOnStatus) > 0, inputs.ResponseParsing != nil, inputs.ResponseEncoding != nil:
		return errors.New("responseMode raw returns every status and the body bytes as is")
	}
	return nil
}

func (h *Handler) runRaw(
	ctx context.Context,
	task *types.Task,
	inputs RequestInputs,
	credential *privateconnection.PrivateCredentials,
) (interface{}, error) {
	target, err := validateRawURL(inputs)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, err.Error())
	}
	inputHeaders := map[string][]string{}
	for _, header := range inputs.RequestHeaders {
		if credential != nil && credential.Type == privateconnection.UserOAuth2TokenAuthType && strings.EqualFold(header.Key, "Authorization") {
			e := errors.New("a user OAuth2 connection takes Authorization only from the secret inputs")
			return nil, util.DefaultActionErrorWithDisplayError(e, e.Error())
		}
		inputHeaders[header.Key] = append(inputHeaders[header.Key], header.Value...)
	}
	if inputs.EncryptedCredentials != "" {
		if err := h.mergeSecretInputs(&inputs, inputHeaders); err != nil {
			return nil, util.DefaultActionErrorWithDisplayError(err, err.Error())
		}
	}
	var responseKey hpke.PublicKey
	if inputs.EncryptResponseTo != nil {
		if responseKey, err = parseResponseKey(*inputs.EncryptResponseTo); err != nil {
			return nil, util.DefaultActionErrorWithDisplayError(err, err.Error())
		}
	}
	body, err := base64.StdEncoding.DecodeString(inputs.BodyBase64)
	if err != nil {
		e := errors.New("bodyBase64 is not valid base64")
		return nil, util.DefaultActionErrorWithDisplayError(e, e.Error())
	}
	connectionHeaders, err := connectionHeadersFor(target, credential)
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, err.Error())
	}

	req, err := http.NewRequestWithContext(ctx, inputs.Verb, inputs.URL, bytes.NewReader(body))
	if err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, "error creating request")
	}
	if len(body) == 0 {
		req.Body = http.NoBody
	}
	if err := setRawRequestHeaders(req, inputHeaders, connectionHeaders, task.Data.Attributes.SecDatadogHeaderValue); err != nil {
		return nil, util.DefaultActionErrorWithDisplayError(err, err.Error())
	}

	client, err := h.httpClientProvider.NewClient(&httpclient.RunnerHttpClientConfig{
		AllowIMDSEndpoints: h.allowIMDSEndpoints,
		HTTPTimeout:        h.httpTimeout,
		DisableRedirects:   true,
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

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, MaxRawResponseBodyBytes+1))
	if err != nil {
		return nil, util.DefaultActionError(errors.New("failed to read response body"))
	}
	truncated := len(respBody) > MaxRawResponseBodyBytes
	if truncated {
		respBody = respBody[:MaxRawResponseBodyBytes]
	}

	headers := resp.Header.Clone()
	for name := range rawStrippedResponseHeaders {
		headers.Del(name)
	}
	output := &RequestOutputs{Status: resp.StatusCode, Headers: headers, BodyTruncated: truncated}
	if responseKey == nil {
		output.BodyBase64 = base64.StdEncoding.EncodeToString(respBody)
		return output, nil
	}
	sealed, err := hpke.Seal(responseKey, hpke.HKDFSHA256(), hpke.AES256GCM(), rawResponseHPKEInfo, respBody)
	if err != nil {
		return nil, util.DefaultActionError(fmt.Errorf("failed to encrypt response body: %w", err))
	}
	output.EncryptedBody = base64.StdEncoding.EncodeToString(sealed)
	return output, nil
}

func (h *Handler) mergeSecretInputs(inputs *RequestInputs, inputHeaders map[string][]string) error {
	if h.encryptionStore == nil {
		return errors.New("secret inputs are not supported by this runner")
	}
	secrets, err := encryptioncontext.DecryptInto[RawSecretInputs](h.encryptionStore, inputs.EncryptionContext, inputs.EncryptedCredentials)
	if err != nil {
		return fmt.Errorf("failed to decrypt secret inputs: %w", err)
	}
	for name, values := range secrets.Headers {
		inputHeaders[name] = append(inputHeaders[name], values...)
	}
	if secrets.BodyBase64 != "" {
		if inputs.BodyBase64 != "" {
			return errors.New("both bodyBase64 and the secret body are set")
		}
		inputs.BodyBase64 = secrets.BodyBase64
	}
	return nil
}

func validateRawURL(inputs RequestInputs) (*url.URL, error) {
	switch inputs.Verb {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
	default:
		return nil, fmt.Errorf("unsupported verb %q", inputs.Verb)
	}
	parsed, err := url.Parse(inputs.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, errors.New("url must be an absolute http or https URL")
	}
	if parsed.User != nil {
		return nil, errors.New("url must not contain user info")
	}
	return parsed, nil
}

func parseResponseKey(key ResponseKey) (hpke.PublicKey, error) {
	if key.KeyType != RawResponseKeyType {
		return nil, fmt.Errorf("unsupported encryptResponseTo.keyType %q (expected %q)", key.KeyType, RawResponseKeyType)
	}
	raw, err := base64.StdEncoding.DecodeString(key.PublicKey)
	if err != nil {
		return nil, errors.New("encryptResponseTo.publicKey is not valid base64")
	}
	publicKey, err := hpke.DHKEM(ecdh.P256()).NewPublicKey(raw)
	if err != nil {
		return nil, errors.New("encryptResponseTo.publicKey is not a valid P-256 public key")
	}
	return publicKey, nil
}

// connectionHeadersFor returns the connection's rendered headers, including its
// credential, when the target is the connection's base URL host, and none for any
// other host. A connection that carries headers must name its host.
//
// A user OAuth2 connection's token is a secret input of the task, not a
// connection header, so it may only be used against the connection's host.
func connectionHeadersFor(target *url.URL, credential *privateconnection.PrivateCredentials) (http.Header, error) {
	headers := http.Header{}
	if credential == nil {
		return headers, nil
	}
	details := credential.HttpDetails
	if credential.Type == privateconnection.UserOAuth2TokenAuthType && (details.BaseURL == "" || !sameOrigin(target, details.BaseURL)) {
		return nil, errors.New("a user OAuth2 connection only sends requests to its base URL")
	}
	hasHeaders := len(details.Headers) > 0 || credential.Type == privateconnection.BasicAuthType
	if details.BaseURL == "" {
		if hasHeaders {
			return nil, errors.New("connection headers require a base URL to scope them to a host")
		}
		return headers, nil
	}
	if !sameOrigin(target, details.BaseURL) {
		return headers, nil
	}

	raw := make(map[string]string, len(details.Headers))
	for _, header := range details.Headers {
		raw[header.Name] = header.Value
	}
	rendered, err := evaluateHeaders(raw, credential)
	if err != nil {
		return nil, errors.New("cannot evaluate variables in connection headers")
	}
	for name, value := range rendered {
		headers.Set(name, value)
	}
	if credential.Type == privateconnection.BasicAuthType {
		username, password, err := credential.GetUsernamePasswordBasicAuth()
		if err != nil {
			return nil, err
		}
		headers.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+password)))
	}
	return headers, nil
}

// sameOrigin compares scheme, host and port. An http base URL also accepts https.
func sameOrigin(target *url.URL, baseURL string) bool {
	base, err := url.Parse(baseURL)
	if err != nil || base.Host == "" {
		return false
	}
	if base.Scheme == "https" && target.Scheme != "https" {
		return false
	}
	return strings.EqualFold(target.Hostname(), base.Hostname()) && effectivePort(target) == effectivePort(base)
}

func effectivePort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if u.Scheme == "http" {
		return "80"
	}
	return "443"
}

// setRawRequestHeaders copies the caller's headers verbatim, without rendering
// templates in them, so a caller cannot read connection secrets through a
// header. Connection headers override the caller's, except that a caller
// Authorization meeting a connection Authorization fails the request: silently
// swapping one credential for another hides misconfiguration.
func setRawRequestHeaders(
	req *http.Request,
	inputHeaders map[string][]string,
	connectionHeaders http.Header,
	secDatadogHeaderValue string,
) error {
	for name, values := range inputHeaders {
		canonical := textproto.CanonicalMIMEHeaderKey(name)
		if rawStrippedRequestHeaders[canonical] {
			continue
		}
		for _, value := range values {
			req.Header.Add(canonical, value)
		}
	}
	if req.Header.Get("Authorization") != "" && connectionHeaders.Get("Authorization") != "" {
		return errors.New("both the request and the connection set Authorization")
	}
	for name, values := range connectionHeaders {
		req.Header[name] = values
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "Datadog")
	}
	req.Header.Set(DdSecurityHeader, secDatadogHeaderValue)
	return nil
}
