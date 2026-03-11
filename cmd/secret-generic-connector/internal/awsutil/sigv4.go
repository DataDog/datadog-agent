// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package awsutil provides lightweight AWS Signature V4 signing and credential
// resolution without depending on the AWS SDK, keeping the binary small.
package awsutil

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// SignRequest signs an HTTP request using AWS Signature Version 4.
// The payload must match the request body that will be sent.
func SignRequest(req *http.Request, creds Credentials, region, service string, payload []byte) {
	now := time.Now().UTC()
	signRequestAt(req, creds, region, service, payload, now)
}

func signRequestAt(req *http.Request, creds Credentials, region, service string, payload []byte, now time.Time) {
	datestamp := now.Format("20060102")
	amzdate := now.Format("20060102T150405Z")

	req.Header.Set("X-Amz-Date", amzdate)
	if creds.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", creds.SessionToken)
	}

	// Host header must be present for signing
	if req.Header.Get("Host") == "" && req.Host != "" {
		req.Header.Set("Host", req.Host)
	} else if req.Header.Get("Host") == "" && req.URL != nil {
		req.Header.Set("Host", req.URL.Host)
	}

	signedHeaders, canonicalHeaders := buildCanonicalHeaders(req)
	payloadHash := sha256Hex(payload)

	canonicalRequest := strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQueryString(req.URL),
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", datestamp, region, service)

	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzdate,
		credentialScope,
		sha256Hex([]byte(canonicalRequest)),
	}, "\n")

	signingKey := deriveSigningKey(creds.SecretAccessKey, datestamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.AccessKeyID, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
}

func buildCanonicalHeaders(req *http.Request) (signedHeaders, canonicalHeaders string) {
	type header struct {
		key string
		val string
	}

	var headers []header
	for k, v := range req.Header {
		lk := strings.ToLower(k)
		headers = append(headers, header{lk, strings.TrimSpace(strings.Join(v, ","))})
	}
	sort.Slice(headers, func(i, j int) bool { return headers[i].key < headers[j].key })

	var signed []string
	var canonical []string
	for _, h := range headers {
		signed = append(signed, h.key)
		canonical = append(canonical, h.key+":"+h.val+"\n")
	}

	signedHeaders = strings.Join(signed, ";")
	canonicalHeaders = strings.Join(canonical, "")
	return
}

func canonicalURI(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func canonicalQueryString(u *url.URL) string {
	query := u.Query()
	keys := make([]string, 0, len(query))
	for k := range query {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		vals := query[k]
		sort.Strings(vals)
		for _, v := range vals {
			parts = append(parts, awsQueryEscape(k)+"="+awsQueryEscape(v))
		}
	}
	return strings.Join(parts, "&")
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

// awsQueryEscape percent-encodes a string per AWS SigV4 rules (spaces as %20, not +).
func awsQueryEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func deriveSigningKey(secret, datestamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(datestamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}
