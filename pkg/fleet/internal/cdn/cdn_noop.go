// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cdn

import (
	"context"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type fetcherNoop struct {
}

// newNoopFetcher creates a new noop CDN.
func newNoopFetcher() (CDNFetcher, error) {
	return &fetcherNoop{}, nil
}

func (c *fetcherNoop) get(_ context.Context) ([][]byte, error) {
	log.Debug("Noop CDN get")
	return nil, nil
}

func (c *fetcherNoop) close() error {
	log.Debug("Noop CDN close")
	return nil
}
