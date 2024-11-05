// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type cdnNoop struct {
}

type configNoop struct{}

// newCDNNoop creates a new noop CDN.
func newCDNNoop() (CDN, error) {
	return &cdnNoop{}, nil
}

// Get gets the configuration from the CDN.
func (c *cdnNoop) Get(_ context.Context, _ string) (Config, error) {
	log.Debug("Noop CDN get")
	return &configNoop{}, nil
}

func (c *cdnNoop) Close() error {
	log.Debug("Noop CDN close")
	return nil
}

func (c *configNoop) Version() string {
	log.Debug("Noop CDN version")
	return ""
}

func (c *configNoop) Write(_ string) error {
	log.Debug("Noop CDN write")
	return nil
}
