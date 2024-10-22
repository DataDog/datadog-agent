// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"context"
)

type cdnNoop struct {
}

type configNoop struct{}

// newNoop creates a new noop CDN.
func newNoop() (CDN, error) {
	return &cdnNoop{}, nil
}

// Get gets the configuration from the CDN.
func (c *cdnNoop) Get(_ context.Context, _ string) (Config, error) {
	return &configNoop{}, nil
}

func (c *cdnNoop) Close() error {
	return nil
}

func (c *configNoop) Version() string {
	return ""
}

func (c *configNoop) Write(_ string) error {
	return nil
}
