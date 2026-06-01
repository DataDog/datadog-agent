// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !python

package python

import (
	"errors"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
)

// DiscoverConfig is unavailable when the Agent is built without Python support.
func DiscoverConfig(_ string, _ DiscoveryService) ([]integration.Config, error) {
	return nil, errors.New("python support is not available")
}
