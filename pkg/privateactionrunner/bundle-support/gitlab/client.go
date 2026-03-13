// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gitlab

import (
	"fmt"
	"strconv"

	"github.com/hashicorp/go-retryablehttp"
	gitlab "gitlab.com/gitlab-org/api/client-go"

	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/privateconnection"
)

const (
	urlTokenName = "baseURL"
	apiTokenName = "gitlabApiToken"

	defaultBaseURL = "https://gitlab.com"
)

// A Number represents a JSON number literal.
type Number string

// String returns the literal text of the number.
func (n Number) String() string { return string(n) }

// Float64 returns the number as a float64.
func (n Number) Float64() (float64, error) {
	return strconv.ParseFloat(string(n), 64)
}

// Int64 returns the number as an int64.
func (n Number) Int64() (int64, error) {
	return strconv.ParseInt(string(n), 10, 64)
}

func NewGitlabClient(credential *privateconnection.PrivateCredentials) (*gitlab.Client, error) {
	credentialTokens := credential.AsTokenMap()
	apiToken := credentialTokens[apiTokenName]
	baseURL := credentialTokens[urlTokenName]
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	git, err := gitlab.NewClient(apiToken, gitlab.WithBaseURL(baseURL))
	if err != nil {
		return nil, fmt.Errorf("could not create the gitlab client: %w", err)
	}
	return git, nil
}

func WithPagination(page, perPage int) gitlab.RequestOptionFunc {
	return func(req *retryablehttp.Request) error {
		q := req.URL.Query()
		q.Add("page", strconv.Itoa(page))
		q.Add("per_page", strconv.Itoa(perPage))
		req.URL.RawQuery = q.Encode()
		return nil
	}
}
