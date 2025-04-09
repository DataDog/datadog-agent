// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

package ipc

import (
	"net/http"
	"net/http/httptest"

	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Mock is the mocked component type.
type Mock interface {
	Component
	// NewMockServer allows to create a mock server that use the IPC certificate
	NewMockServer(handler http.Handler) *httptest.Server
	Optional() option.Option[Component]
}
