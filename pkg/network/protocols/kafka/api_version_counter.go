// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	"fmt"
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

const (
	MinSupportedApiVersion = 1
	MaxSupportedApiVersion = 11
)

// APIVersionCounter is a Kafka API version aware counter, it has a counter for each supported Kafka API version.
// It enables the use of a single metric that increments based on the API version, avoiding the need for separate metrics for API version
type APIVersionCounter struct {
	hitsVersions           [MaxSupportedApiVersion]*libtelemetry.Counter
	hitsUnsupportedVersion *libtelemetry.Counter
}

// NewAPIVersionCounter creates and returns a new instance of APIVersionCounter
func NewAPIVersionCounter(metricGroup *libtelemetry.MetricGroup, metricName string, tags ...string) *APIVersionCounter {
	var hitsVersions [MaxSupportedApiVersion]*libtelemetry.Counter
	for i := 0; i < MaxSupportedApiVersion; i++ {
		hitsVersions[i] = metricGroup.NewCounter(metricName, append(tags, fmt.Sprintf("protocol_version:%d", i+1))...)
	}
	return &APIVersionCounter{
		hitsVersions:           hitsVersions,
		hitsUnsupportedVersion: metricGroup.NewCounter(metricName, append(tags, "protocol_version:unsupported")...),
	}
}

// Add increments the APIVersion counter based on the specified request api version
func (c *APIVersionCounter) Add(tx *EbpfTx) {
	if tx.Request_api_version < MinSupportedApiVersion || tx.Request_api_version > MaxSupportedApiVersion {
		c.hitsUnsupportedVersion.Add(1)
		return
	}
	c.hitsVersions[tx.Request_api_version-1].Add(1)
}
