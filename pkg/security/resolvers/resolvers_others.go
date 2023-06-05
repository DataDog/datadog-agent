// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package resolvers

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-go/v5/statsd"
)

// Resolvers holds the list of the event attribute resolvers
type Resolvers struct {
}

// NewResolvers creates a new instance of Resolvers
func NewResolvers(config *config.Config, statsdClient statsd.ClientInterface) (*Resolvers, error) {

	return nil, fmt.Errorf("Not implemented on this platform")
}
