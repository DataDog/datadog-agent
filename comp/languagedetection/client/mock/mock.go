// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the language detection client component.
package mock

import (
	"testing"

	client "github.com/DataDog/datadog-agent/comp/languagedetection/client/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type mockClient struct{}

// Mock returns a mock for the language detection client component.
func Mock(_ *testing.T) option.Option[client.Component] {
	return option.New[client.Component](&mockClient{})
}
