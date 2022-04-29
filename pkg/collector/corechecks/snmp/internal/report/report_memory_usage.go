// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package report

import (
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func (ms *MetricSender) trySendMemoryUsageMetric(symbol checkconfig.SymbolConfig, fullIndex string, values *valuestore.ResultValueStore, tags []string) {
	err := ms.sendMemoryUsageMetric(symbol, fullIndex, values, tags)
	if err != nil {
		log.Debugf("failed to send bandwidth usage metric: %s", err)
	}
}

func (ms *MetricSender) sendMemoryUsageMetric(symbol checkconfig.SymbolConfig, fullIndex string, values *valuestore.ResultValueStore, tags []string) error {
	shouldGenerateMemoryMetric(fullIndex, values)
	return nil
}


func shouldGenerateMemoryMetric(values) {
	ms.metricNameToOidMap[]
	if memory.used && memory.available {
		return memory.used
	}

}
