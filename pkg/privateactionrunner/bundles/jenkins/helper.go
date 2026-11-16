// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_jenkins

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

type DomainAndHeaders struct {
	Domain  string
	Headers http.Header
}

// getHeadersAndDomain generates headers and domain from credentials
func getHeadersAndDomain(credentials *privateconnection.PrivateCredentials) (DomainAndHeaders, error) {
	credentialTokens := credentials.AsTokenMap()

	userName := credentialTokens["username"]
	apiToken := credentialTokens["token"]
	domain := credentialTokens["domain"]
	// Check if essential fields are populated
	if userName == "" || apiToken == "" || domain == "" {
		return DomainAndHeaders{}, errors.New("missing required credentials: username, token, or domain")
	}

	return DomainAndHeaders{
		Domain:  domain,
		Headers: createHeaders(userName, apiToken),
	}, nil
}

func createHeaders(userName, apiToken string) http.Header {
	encodedCreds := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", userName, apiToken)))

	headers := http.Header{}
	headers.Set("Accept", "application/json")
	headers.Set("Content-Type", "application/json")
	headers.Set("Authorization", "Basic "+encodedCreds)

	return headers
}

// Dot segments are rejected because url.PathEscape leaves "." unescaped, so "../" could escape /job/.
func encodeJobNameForUrl(jobName string) (string, error) {
	if jobName == "" {
		return "", errors.New("job name must not be empty")
	}
	segments := strings.Split(jobName, "/")
	for i, segment := range segments {
		if segment == "." || segment == ".." {
			return "", fmt.Errorf("job name %q must not contain %q path segments", jobName, segment)
		}
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/"), nil
}
