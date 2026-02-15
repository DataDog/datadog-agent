// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package opms

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	app "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/constants"
	log "github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/logging"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/modes"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/adapters/parversion"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/libs/par"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/util"
	"github.com/DataDog/jsonapi"
	"github.com/go-jose/go-jose/v4"
)

const (
	createPARPath = "/api/unstable/on_prem_runners"
)

// PublicClient exposes endpoint that don't require JWT authentication
type PublicClient interface {
	EnrollWithApiKey(
		ctx context.Context,
		apiKey string,
		appKey string,
		runnerName string,
		runnerModes []modes.Mode,
		publicJwk *jose.JSONWebKey,
	) (*par.CreateRunnerResponse, error)
}

type publicClient struct {
	ddApiHost  string
	httpClient *http.Client
}

func NewPublicClient(ddBaseURL string) PublicClient {
	apiHost := strings.Replace(ddBaseURL, "https://", "", 1)
	return &publicClient{
		ddApiHost: apiHost,
		httpClient: &http.Client{
			Timeout: time.Millisecond * time.Duration(30_000),
		},
	}
}

func (p *publicClient) EnrollWithApiKey(
	ctx context.Context,
	apiKey string,
	appKey string,
	runnerName string,
	runnerModes []modes.Mode,
	publicJwk *jose.JSONWebKey,
) (*par.CreateRunnerResponse, error) {
	publicKeyPEM, err := util.JWKToPEM(publicJwk)
	if err != nil {
		return nil, fmt.Errorf("failed to convert public key to PEM: %w", err)
	}

	createRunnerUrl := url.URL{
		Host:   p.ddApiHost,
		Scheme: "https",
		Path:   createPARPath,
	}

	request := par.CreateRunnerRequest{
		RunnerName:   runnerName,
		RunnerModes:  runnerModes,
		PublicKeyPEM: publicKeyPEM,
	}

	requestBodyJSON, err := jsonapi.Marshal(request, jsonapi.MarshalClientMode())
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", createRunnerUrl.String(), strings.NewReader(string(requestBodyJSON)))
	if err != nil {
		return nil, fmt.Errorf("failed to build runner creation request: %w", err)
	}

	req.Header.Set("Content-Type", "application/vnd.api+json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	req.Header.Set("DD-APPLICATION-KEY", appKey)
	req.Header.Set(app.VersionHeaderName, parversion.RunnerVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send runner creation request: %w", err)
	}
	defer func() {
		err = resp.Body.Close()
		if err != nil {
			log.Error("error closing runner creation response body", log.ErrorField(err))
		}
	}()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("runner creation failed with HTTP status code %d and failed to read HTTP response with error %w", resp.StatusCode, err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("runner creation failed with HTTP status code %d and response %s", resp.StatusCode, string(respBody))
	}

	createRunnerResponse := new(par.CreateRunnerResponse)
	err = jsonapi.Unmarshal(respBody, createRunnerResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal runner creation response: %w", err)
	}
	return createRunnerResponse, nil
}
