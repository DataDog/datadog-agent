// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package mock

import (
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
)

// Filterable implements the workloadfilter#Filterable interface for testing
type Filterable struct {
	EntityName        string
	EntityAnnotations map[string]string
	EntityType        workloadfilter.ResourceType
}

var _ workloadfilter.Filterable = &Filterable{}

// Serialize implements Filterable#Serialize
func (m *Filterable) Serialize() any {
	return nil
}

// Type implements Filterable#Type
func (m *Filterable) Type() workloadfilter.ResourceType {
	if m.EntityType != "" {
		return m.EntityType
	}
	// Default to container if not set, or could be "mock" depending on usage
	return workloadfilter.ResourceType("mock")
}

// GetAnnotations implements Filterable#GetAnnotations
func (m *Filterable) GetAnnotations() map[string]string {
	return m.EntityAnnotations
}

// GetName implements Filterable#GetName
func (m *Filterable) GetName() string {
	return m.EntityName
}

// ToBytes implements Filterable#ToBytes
func (m *Filterable) ToBytes() ([]byte, error) {
	return nil, nil
}
