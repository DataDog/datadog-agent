// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux

// Package file holds file related files
package file

import (
	"errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// Opt defines options for resolvers
type Opt struct{}

// Resolver represents a cache for mountpoints and the corresponding file systems
type Resolver struct {
}

// NewResolver returns a new instance of the hash resolver
func NewResolver(_ statsd.ClientInterface, _ *Opt) (*Resolver, error) {
	return &Resolver{}, errors.New("not yet implemented for other OS")
}

// ResolveFileMetadata resolves file metadata
func (r *Resolver) ResolveFileMetadata(_ *model.Event, _ *model.FileEvent) (*model.FileMetadata, error) {
	return nil, errors.New("not yet implemented for other OS")
}

// SendStats sends the resolver metrics
func (r *Resolver) SendStats() error {
	return nil
}
