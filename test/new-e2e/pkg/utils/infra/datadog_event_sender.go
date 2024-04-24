// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package infra implements utilities to interact with a Pulumi infrastructure
package infra

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner"
	"github.com/DataDog/datadog-agent/test/new-e2e/pkg/runner/parameters"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
)

type datadogEventSender interface {
	SendEvent(body datadogV1.EventCreateRequest)
}

type datadogEventSenderImpl struct {
	ctx       context.Context
	eventsAPI *datadogV1.EventsApi

	logger io.Writer
}

var _ datadogEventSender = &datadogEventSenderImpl{}

func newDatadogEventSender(logger io.Writer) (*datadogEventSenderImpl, error) {
	apiKey, err := runner.GetProfile().SecretStore().GetWithDefault(parameters.APIKey, "")
	if err != nil {
		fmt.Fprintf(logger, "error when getting API key from parameter store: %v", err)
		return nil, err
	}

	if apiKey == "" {
		fmt.Fprintf(logger, "Skipping sending event because API key is empty")
		return nil, errors.New("empty API key")
	}

	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)
	eventsAPI := datadogV1.NewEventsApi(apiClient)
	return &datadogEventSenderImpl{
		ctx: context.WithValue(context.Background(), datadog.ContextAPIKeys, map[string]datadog.APIKey{
			"apiKeyAuth": {
				Key: apiKey,
			},
		}),
		eventsAPI: eventsAPI,
		logger:    logger,
	}, nil
}

func (d *datadogEventSenderImpl) SendEvent(body datadogV1.EventCreateRequest) {
	_, response, err := d.eventsAPI.CreateEvent(d.ctx, body)

	if err != nil {
		fmt.Fprintf(d.logger, "error when calling `EventsApi.CreateEvent`: %v", err)
		fmt.Fprintf(d.logger, "Full HTTP response: %v\n", response)
		return
	}
}
