// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the serializer component.
package mock

import (
	"testing"

	serializerdef "github.com/DataDog/datadog-agent/comp/serializer/serializer/def"
	"github.com/DataDog/datadog-agent/pkg/serializer/mocks"
)

var _ serializerdef.Component = (*mocks.MetricSerializer)(nil)

// New returns a mock that implements the serializer component and each
// of its functional group interfaces.
func New(t *testing.T) *mocks.MetricSerializer {
	return mocks.NewMetricSerializer(t)
}
