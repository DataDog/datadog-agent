// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the statsd component.
package mock

import (
	"testing"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"

	statsd "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/def"
)

type mockService struct {
	client ddgostatsd.ClientInterface
}

func (m *mockService) Get() (ddgostatsd.ClientInterface, error) {
	return m.client, nil
}

func (m *mockService) Create(_ ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return m.client, nil
}

func (m *mockService) CreateForAddr(_ string, _ ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return m.client, nil
}

func (m *mockService) CreateForHostPort(_ string, _ int, _ ...ddgostatsd.Option) (ddgostatsd.ClientInterface, error) {
	return m.client, nil
}

var _ statsd.Component = (*mockService)(nil)

// Mock returns a mock for statsd component.
func Mock(_ *testing.T) statsd.Component {
	return &mockService{client: &ddgostatsd.NoOpClient{}}
}
