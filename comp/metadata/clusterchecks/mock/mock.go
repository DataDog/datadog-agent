// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package mock provides a mock implementation of the clusterchecks metadata component
package mock

import (
	"net/http"

	clusterchecksmetadata "github.com/DataDog/datadog-agent/comp/metadata/clusterchecks/def"
)

// ClusterChecksMock is a mock implementation of the clusterchecks.Component
type ClusterChecksMock struct{}

// SetClusterHandler is a no-op for the mock
func (cc *ClusterChecksMock) SetClusterHandler(_ interface{}) {
	// No-op
}

// WritePayloadAsJSON is a no-op for the mock
func (cc *ClusterChecksMock) WritePayloadAsJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"mock": "cluster checks metadata"}`))
}

// NewMock creates a new mock clusterchecks component
func NewMock() clusterchecksmetadata.Component {
	return &ClusterChecksMock{}
}
