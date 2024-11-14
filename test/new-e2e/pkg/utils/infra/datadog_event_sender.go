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
	"sync"

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

	initOnce sync.Once
	isReady  bool
}

var _ datadogEventSender = &datadogEventSenderImpl{}

func newDatadogEventSender(logger io.Writer) *datadogEventSenderImpl {
	return &datadogEventSenderImpl{
		logger:   logger,
		initOnce: sync.Once{},
		isReady:  false,
	}
}

func (d *datadogEventSenderImpl) initDatadogEventSender() error {
	apiKey, err := runner.GetProfile().SecretStore().GetWithDefault(parameters.APIKey, "")
	if err != nil {
		fmt.Fprintf(d.logger, "error when getting API key from parameter store: %v", err)
		return err
	}

	if apiKey == "" {
		fmt.Fprintf(d.logger, "Skipping sending event because API key is empty")
		return errors.New("empty API key")
	}

	d.ctx = context.WithValue(context.Background(), datadog.ContextAPIKeys, map[string]datadog.APIKey{
		"apiKeyAuth": {
			Key: apiKey,
		},
	})

	configuration := datadog.NewConfiguration()
	apiClient := datadog.NewAPIClient(configuration)
	eventsAPI := datadogV1.NewEventsApi(apiClient)

	d.eventsAPI = eventsAPI

	d.isReady = true

	return nil
}

func (d *datadogEventSenderImpl) SendEvent(body datadogV1.EventCreateRequest) {
	d.initOnce.Do(func() {
		err := d.initDatadogEventSender()
		if err != nil {
			fmt.Fprintf(d.logger, "error when initializing `datadogEventSender`: %v", err)
			d.isReady = false
		}
	})

	if !d.isReady {
		return
	}

	_, response, err := d.eventsAPI.CreateEvent(d.ctx, body)

	if err != nil {
		fmt.Fprintf(d.logger, "error when calling `EventsApi.CreateEvent`: %v", err)
		fmt.Fprintf(d.logger, "Full HTTP response: %v\n", response)
		return
	}
}
