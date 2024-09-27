// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package types defines types used by the Tagger component.
package types

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDefaultEntityID_GetID(t *testing.T) {
	tests := []struct {
		name       string
		entityID   EntityID
		expectedID string
	}{
		{
			name:       "invalid format, not containing `://`",
			entityID:   newDefaultEntityID("invalid_entity_id"),
			expectedID: "",
		},
		{
			name:       "invalid format, multiple `://`",
			entityID:   newDefaultEntityID("invalid://entity://id"),
			expectedID: "entity://id",
		},
		{
			name:       "conforming format, single `://`",
			entityID:   newDefaultEntityID("good://format"),
			expectedID: "format",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			assert.Equal(tt, test.expectedID, test.entityID.GetID())
		})
	}
}

func TestDefaultEntityID_GetPrefix(t *testing.T) {
	tests := []struct {
		name           string
		entityID       EntityID
		expectedPrefix EntityIDPrefix
	}{
		{
			name:           "invalid format, not containing `://`",
			entityID:       newDefaultEntityID("invalid_entity_id"),
			expectedPrefix: "",
		},
		{
			name:           "invalid format, multiple `://`",
			entityID:       newDefaultEntityID("invalid://entity://id"),
			expectedPrefix: "invalid",
		},
		{
			name:           "conforming format, single `://`",
			entityID:       newDefaultEntityID("good://format"),
			expectedPrefix: "good",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(tt *testing.T) {
			assert.Equal(tt, test.expectedPrefix, test.entityID.GetPrefix())
		})
	}
}
