// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build docker || (cri && containerd) || test

package fx

import (
	configfilesdiscoveryimpl "github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl"
	"github.com/DataDog/datadog-agent/comp/core/configfilesdiscovery/impl/collectors"
)

func newDefaultCollectors() map[string]configfilesdiscoveryimpl.ConfigCollector {
	return map[string]configfilesdiscoveryimpl.ConfigCollector{
		collectors.KafkaIntegrationName: collectors.NewKafka(),
		collectors.RedisIntegrationName: collectors.NewRedis(),
	}
}
