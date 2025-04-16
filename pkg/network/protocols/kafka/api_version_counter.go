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

// apiVersionCounter is a Kafka API version aware counter, it has a counter for each supported Kafka API version.
// It enables the use of a single metric that increments based on the API version, avoiding the need for separate metrics for API version
type apiVersionCounter struct {
	hitsVersions           []*libtelemetry.Counter
	hitsUnsupportedVersion *libtelemetry.Counter
	minSupportedVersion    uint16
	maxSupportedVersion    uint16
}

// newAPIVersionCounter creates and returns a new instance of apiVersionCounter
func newAPIVersionCounter(metricGroup *libtelemetry.MetricGroup, metricName string, minSupportedVersion, maxSupportedVersion uint16, tags ...string) *apiVersionCounter {
	hitsVersions := make([]*libtelemetry.Counter, maxSupportedVersion-minSupportedVersion+1)
	for i := minSupportedVersion; i <= maxSupportedVersion; i++ {
		hitsVersions[i-minSupportedVersion] = metricGroup.NewCounter(metricName, append(tags, fmt.Sprintf("protocol_version:%d", i))...)
	}
	return &apiVersionCounter{
		hitsVersions:           hitsVersions,
		hitsUnsupportedVersion: metricGroup.NewCounter(metricName, append(tags, "protocol_version:unsupported")...),
		minSupportedVersion:    minSupportedVersion,
		maxSupportedVersion:    maxSupportedVersion,
	}
}

// Add increments the API version counter based on the specified request api version
func (c *apiVersionCounter) Add(tx *KafkaTransaction) {
	currentVersion := uint16(tx.Request_api_version)
	if currentVersion < c.minSupportedVersion || currentVersion > c.maxSupportedVersion {
		c.hitsUnsupportedVersion.Add(int64(tx.Records_count))
		return
	}
	c.hitsVersions[currentVersion-c.minSupportedVersion].Add(int64(tx.Records_count))
}
