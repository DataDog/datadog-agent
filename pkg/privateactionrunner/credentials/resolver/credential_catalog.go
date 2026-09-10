// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package resolver

import (
	"errors"
	"sort"
)

// CredentialCatalog contains only credentials an operator explicitly made
// available to the Private Action Runner. Keys, unlike values, are safe to send
// to the backend.
type CredentialCatalog struct {
	configured map[string]string
}

// CredentialDescriptor is the non-secret portion of a catalog entry.
type CredentialDescriptor struct {
	Key    string
	Source string
}

// NewCredentialCatalog builds a catalog from credentials explicitly made
// available in the Private Action Runner configuration.
func NewCredentialCatalog(configured map[string]string) *CredentialCatalog {
	values := make(map[string]string, len(configured))
	for key, value := range configured {
		values[key] = value
	}
	return &CredentialCatalog{configured: values}
}

// Resolve returns the value for an operator-exported credential key.
func (c *CredentialCatalog) Resolve(key string) (string, error) {
	if key == "" {
		return "", errors.New("runner credential key must not be empty")
	}
	value, found := c.snapshot()[key]
	if !found {
		return "", errors.New("requested runner credential is not available")
	}
	return value, nil
}

func (c *CredentialCatalog) snapshot() map[string]string {
	values := make(map[string]string, len(c.configured))
	for key, value := range c.configured {
		values[key] = value
	}
	return values
}

// List returns catalog metadata without secret values, handles, or config paths.
func (c *CredentialCatalog) List() []CredentialDescriptor {
	values := c.snapshot()
	descriptors := make([]CredentialDescriptor, 0, len(values))
	for key := range values {
		descriptors = append(descriptors, CredentialDescriptor{Key: key, Source: "configured"})
	}
	sort.Slice(descriptors, func(i, j int) bool { return descriptors[i].Key < descriptors[j].Key })
	return descriptors
}
