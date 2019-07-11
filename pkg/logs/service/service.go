// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package service

import (
	"github.com/DataDog/datadog-agent/pkg/util/containers"
)

// CreationTime represents the moment when the service was created compared to the agent start.
type CreationTime int

const (
	// Before the agent start.
	Before CreationTime = iota
	// After the agent start.
	After
)

// Service represents a process to tail logs for.
type Service struct {
	Type         string
	Identifier   string
	CreationTime CreationTime
}

// NewService returns a new service.
func NewService(providerType string, identifier string, createdTime CreationTime) *Service {
	return &Service{
		Type:         providerType,
		Identifier:   identifier,
		CreationTime: createdTime,
	}
}

// GetEntityID return the entity identifier of the service
func (s *Service) GetEntityID() string {
	return containers.ContainerEntityPrefix + s.Identifier
}
