// Unless explicitly stated otherwise all files in this repository are licensed
// under the BSD 3-Clause License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package awsutil provides AWS utility functions for signing requests.
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

	"github.com/aws/aws-sdk-go-v2/aws"
)

// SignRequest signs an HTTP request with AWS SigV4 using the given credentials.
func SignRequest(req *http.Request, creds aws.Credentials, region, service string, body []byte) {
	signRequestAt(req, creds, region, service, body, time.Now().UTC())
}

func signRequestAt(req *http.Request, creds aws.Credentials, region, service string, body []byte, t time.Time) {
	datestamp := t.Format("20060102")
	amzDate := t.Format("20060102T150405Z")

	req.Header.Set("X-Amz-Date", amzDate)
	if creds.SessionToken != "" {
		req.Header.Set("X-Amz-Security-Token", creds.SessionToken)
	}

	payloadHash := sha256Hex(body)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	signedHeaders, signedHeaderStr := buildSignedHeaders(req)
	canonicalRequest := buildCanonicalRequest(req, signedHeaders, signedHeaderStr, payloadHash)

	credentialScope := fmt.Sprintf("%s/%s/%s/aws4_request", datestamp, region, service)
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate, credentialScope, sha256Hex([]byte(canonicalRequest)))

	signingKey := deriveSigningKey(creds.SecretAccessKey, datestamp, region, service)
	signature := hex.EncodeToString(hmacSHA256(signingKey, []byte(stringToSign)))

	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		creds.AccessKeyID, credentialScope, signedHeaderStr, signature)
	req.Header.Set("Authorization", authHeader)
}

func buildSignedHeaders(req *http.Request) ([]string, string) {
	var keys []string
	for k := range req.Header {
		keys = append(keys, strings.ToLower(k))
	}
	keys = append(keys, "host")
	sort.Strings(keys)
	return keys, strings.Join(keys, ";")
}

func buildCanonicalRequest(req *http.Request, signedHeaders []string, signedHeaderStr, payloadHash string) string {
	var headerLines []string
	for _, k := range signedHeaders {
		var v string
		if k == "host" {
			v = req.Host
			if v == "" {
				v = req.URL.Host
			}
		} else {
			v = req.Header.Get(k)
		}
		headerLines = append(headerLines, k+":"+strings.TrimSpace(v))
	}
	return strings.Join([]string{
		req.Method,
		canonicalURI(req.URL),
		canonicalQueryString(req.URL),
		strings.Join(headerLines, "\n") + "\n",
		signedHeaderStr,
		payloadHash,
	}, "\n")
}

func canonicalURI(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func canonicalQueryString(u *url.URL) string {
	params := u.Query()
	if len(params) == 0 {
		return ""
	}
	var pairs []string
	for k, vals := range params {
		for _, v := range vals {
			pairs = append(pairs, awsQueryEscape(k)+"="+awsQueryEscape(v))
		}
	}
	sort.Strings(pairs)
	return strings.Join(pairs, "&")
}

// awsQueryEscape encodes a string per RFC 3986 (spaces as %20, not +).
func awsQueryEscape(s string) string {
	return strings.ReplaceAll(url.QueryEscape(s), "+", "%20")
}

func deriveSigningKey(secret, datestamp, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), []byte(datestamp))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(service))
	return hmacSHA256(kService, []byte("aws4_request"))
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func sha256Hex(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// ServiceEndpoint returns the endpoint for an AWS service in the given region.
func ServiceEndpoint(service, region string) string {
	if strings.HasPrefix(region, "cn-") {
		return fmt.Sprintf("https://%s.%s.amazonaws.com.cn/", service, region)
	}
	return fmt.Sprintf("https://%s.%s.amazonaws.com/", service, region)
}
