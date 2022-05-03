// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package service

// Service represents a process to tail logs for.
type Service struct {
	Type       string
	Identifier string
}

// NewService returns a new service.
func NewService(providerType string, identifier string) *Service {
	return &Service{
		Type:       providerType,
		Identifier: identifier,
	}
}

// GetEntityID return the entity identifier of the service
func (s *Service) GetEntityID() string {
	return s.Type + "://" + s.Identifier
}
