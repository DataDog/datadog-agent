// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"

	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
)

// StaticHostnameService is a hostname service that returns a fixed hostname
type StaticHostnameService struct {
	hostname string
}

// NewStaticHostnameService creates a new hostname service that returns the provided hostname
func NewStaticHostnameService(hostname string) *StaticHostnameService {
	return &StaticHostnameService{
		hostname: hostname,
	}
}

// Get returns the fixed hostname
func (s *StaticHostnameService) Get(_ context.Context) (string, error) {
	return s.hostname, nil
}

// GetWithProvider returns the fixed hostname with "static" as the provider
func (s *StaticHostnameService) GetWithProvider(_ context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{
		Hostname: s.hostname,
		Provider: "static",
	}, nil
}

// GetSafe returns the fixed hostname
func (s *StaticHostnameService) GetSafe(_ context.Context) string {
	return s.hostname
}
