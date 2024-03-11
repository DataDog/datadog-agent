// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package tcp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
)

func TestDestinationHA(t *testing.T) {
	variants := []bool{true, false}
	for _, variant := range variants {
		endpoint := config.Endpoint{
			IsHA: variant,
		}
		isEndpointHA := endpoint.IsHA

		dest := NewDestination(endpoint, false, client.NewDestinationsContext(), false, statusinterface.NewStatusProviderMock())
		isDestHA := dest.IsHA()

		assert.Equal(t, isEndpointHA, isDestHA)
	}
}
