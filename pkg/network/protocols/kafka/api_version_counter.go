// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package kafka

import (
	libtelemetry "github.com/DataDog/datadog-agent/pkg/network/protocols/telemetry"
)

// APIVersionCounter is a Kafka API version aware counter, it has a counter for each supported Kafka API version.
// It enables the use of a single metric that increments based on the API version, avoiding the need for separate metrics for API version
type APIVersionCounter struct {
	hitsV1, hitsV2, hitsV3, hitsV4, hitsV5, hitsV6, hitsV7, hitsV8, hitsV9, hitsV10, hitsV11 *libtelemetry.Counter
}

// NewAPIVersionCounter creates and returns a new instance of APIVersionCounter
func NewAPIVersionCounter(metricGroup *libtelemetry.MetricGroup, metricName string, tags ...string) *APIVersionCounter {
	return &APIVersionCounter{
		hitsV1:  metricGroup.NewCounter(metricName, append(tags, "protocol_version:1")...),
		hitsV2:  metricGroup.NewCounter(metricName, append(tags, "protocol_version:2")...),
		hitsV3:  metricGroup.NewCounter(metricName, append(tags, "protocol_version:3")...),
		hitsV4:  metricGroup.NewCounter(metricName, append(tags, "protocol_version:4")...),
		hitsV5:  metricGroup.NewCounter(metricName, append(tags, "protocol_version:5")...),
		hitsV6:  metricGroup.NewCounter(metricName, append(tags, "protocol_version:6")...),
		hitsV7:  metricGroup.NewCounter(metricName, append(tags, "protocol_version:7")...),
		hitsV8:  metricGroup.NewCounter(metricName, append(tags, "protocol_version:8")...),
		hitsV9:  metricGroup.NewCounter(metricName, append(tags, "protocol_version:9")...),
		hitsV10: metricGroup.NewCounter(metricName, append(tags, "protocol_version:10")...),
		hitsV11: metricGroup.NewCounter(metricName, append(tags, "protocol_version:11")...),
	}
}

// Add increments the APIVersion counter based on the specified request api version
func (c *APIVersionCounter) Add(tx *EbpfTx) {
	switch tx.Request_api_version {
	case 1:
		c.hitsV1.Add(1)
	case 2:
		c.hitsV2.Add(1)
	case 3:
		c.hitsV3.Add(1)
	case 4:
		c.hitsV4.Add(1)
	case 5:
		c.hitsV5.Add(1)
	case 6:
		c.hitsV6.Add(1)
	case 7:
		c.hitsV7.Add(1)
	case 8:
		c.hitsV8.Add(1)
	case 9:
		c.hitsV9.Add(1)
	case 10:
		c.hitsV10.Add(1)
	case 11:
		c.hitsV11.Add(1)
	}
}
