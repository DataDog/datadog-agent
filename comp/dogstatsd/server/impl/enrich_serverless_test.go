// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build serverless

package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"
)

func TestConvertParseDistributionServerless(t *testing.T) {
	defaultHostname, err := hostname.Get(context.Background())

	assert.Equal(t, "", defaultHostname, "In serverless mode, the hostname returned should be an empty string")
	assert.NoError(t, err)
	parsed, err := parseAndEnrichSingleMetricMessage(t, []byte("daemon:3.5|d"), enrichConfig{})

	assert.NoError(t, err)

	assert.Equal(t, "daemon", parsed.Name)
	assert.Equal(t, 3.5, parsed.Value)
	assert.Equal(t, metrics.DistributionType, parsed.Mtype)
	assert.Equal(t, 0, len(parsed.Tags))

	// this is the important part of the test: hostname.Get() should return
	// an empty string and the parser / enricher should keep the host that way.
	assert.Equal(t, "", parsed.Host, "In serverless mode, the hostname should be an empty string")
}
