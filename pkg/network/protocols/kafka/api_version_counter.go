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
	minSupportedAPIVersion = 1
	maxSupportedAPIVersion = max(MaxSupportedProduceRequestApiVersion, MaxSupportedFetchRequestApiVersion)
)

// apiVersionCounter is a Kafka API version aware counter, it has a counter for each supported Kafka API version.
// It enables the use of a single metric that increments based on the API version, avoiding the need for separate metrics for API version
type apiVersionCounter struct {
	hitsVersions           [maxSupportedAPIVersion]*libtelemetry.Counter
	hitsUnsupportedVersion *libtelemetry.Counter
}

// newAPIVersionCounter creates and returns a new instance of apiVersionCounter
func newAPIVersionCounter(metricGroup *libtelemetry.MetricGroup, metricName string, tags ...string) *apiVersionCounter {
	var hitsVersions [maxSupportedAPIVersion]*libtelemetry.Counter
	for i := 0; i < len(hitsVersions); i++ {
		hitsVersions[i] = metricGroup.NewCounter(metricName, append(tags, fmt.Sprintf("protocol_version:%d", i+1))...)
	}
	return &apiVersionCounter{
		hitsVersions:           hitsVersions,
		hitsUnsupportedVersion: metricGroup.NewCounter(metricName, append(tags, "protocol_version:unsupported")...),
	}
}

// Add increments the API version counter based on the specified request api version
func (c *apiVersionCounter) Add(tx *KafkaTransaction) {
	if tx.Request_api_version < minSupportedAPIVersion || tx.Request_api_version > maxSupportedAPIVersion {
		c.hitsUnsupportedVersion.Add(int64(tx.Records_count))
		return
	}
	c.hitsVersions[tx.Request_api_version-1].Add(int64(tx.Records_count))
}
