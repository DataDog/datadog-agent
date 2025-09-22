package com_datadoghq_jenkins

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

type DomainAndHeaders struct {
	Domain  string
	Headers http.Header
}

// getHeadersAndDomain generates headers and domain from credentials
func getHeadersAndDomain(credentials interface{}) (DomainAndHeaders, error) {
	return DomainAndHeaders{}, fmt.Errorf("not implemented") // TODO
}

func createHeaders(userName, apiToken string) http.Header {
	encodedCreds := base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", userName, apiToken)))

	headers := http.Header{}
	headers.Set("Accept", "application/json")
	headers.Set("Content-Type", "application/json")
	headers.Set("Authorization", fmt.Sprintf("Basic %s", encodedCreds))

	return headers
}

func encodeJobNameForUrl(jobName string) string {
	return strings.ReplaceAll(url.PathEscape(jobName), "%2F", "/")
}
