// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fetch

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/snmp/batchsize"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const failuresWindowDuration = 60 * time.Minute

// OidBatchSizeOptimizers holds a batch size Optimizer for each SNMP request operation.
type OidBatchSizeOptimizers struct {
	snmpGetOptimizer     *batchsize.Optimizer
	snmpGetBulkOptimizer *batchsize.Optimizer
	snmpGetNextOptimizer *batchsize.Optimizer
	lastRefreshTs        time.Time
}

// NewOidBatchSizeOptimizers creates a OidBatchSizeOptimizers.
func NewOidBatchSizeOptimizers(configBatchSize int) *OidBatchSizeOptimizers {
	return &OidBatchSizeOptimizers{
		snmpGetOptimizer:     batchsize.NewOptimizer(configBatchSize),
		snmpGetBulkOptimizer: batchsize.NewOptimizer(configBatchSize),
		snmpGetNextOptimizer: batchsize.NewOptimizer(configBatchSize),
		lastRefreshTs:        time.Now(),
	}
}

// refreshIfOutdated clears each per-operation failure window when the
// configured window has elapsed since the last refresh.
func (o *OidBatchSizeOptimizers) refreshIfOutdated(now time.Time) {
	if now.Sub(o.lastRefreshTs) < failuresWindowDuration {
		return
	}

	o.snmpGetOptimizer.RefreshFailures()
	o.snmpGetBulkOptimizer.RefreshFailures()
	o.snmpGetNextOptimizer.RefreshFailures()

	o.lastRefreshTs = now

	log.Debug("SNMP batch size optimizers have been refreshed")
}
