// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package fetch

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	onSuccessIncreaseValue  = 1
	onFailureDecreaseFactor = 2

	failuresWindowDuration = 60 * time.Minute
	maxFailuresPerWindow   = 2
)

// OidBatchSizeOptimizers holds oidBatchSizeOptimizer for each SNMP request operation
type OidBatchSizeOptimizers struct {
	snmpGetOptimizer     *oidBatchSizeOptimizer
	snmpGetBulkOptimizer *oidBatchSizeOptimizer
	snmpGetNextOptimizer *oidBatchSizeOptimizer
	lastRefreshTs        time.Time
}

// oidBatchSizeOptimizer holds data between check runs to be able to find an optimized batch size for SNMP requests
type oidBatchSizeOptimizer struct {
	snmpOperation           snmpOperation
	configBatchSize         int
	batchSize               int
	failuresByBatchSize     map[int]int
	lastSuccessfulBatchSize int
}

// NewOidBatchSizeOptimizers creates a OidBatchSizeOptimizers
func NewOidBatchSizeOptimizers(configBatchSize int) *OidBatchSizeOptimizers {
	now := time.Now()

	return &OidBatchSizeOptimizers{
		snmpGetOptimizer:     newOidBatchSizeOptimizer(snmpGet, configBatchSize),
		snmpGetBulkOptimizer: newOidBatchSizeOptimizer(snmpGetBulk, configBatchSize),
		snmpGetNextOptimizer: newOidBatchSizeOptimizer(snmpGetNext, configBatchSize),
		lastRefreshTs:        now,
	}
}

// Refresh refreshes each oidBatchSizeOptimizer in OidBatchSizeOptimizers when outdated
func (o *OidBatchSizeOptimizers) refreshIfOutdated(now time.Time) {
	if now.Sub(o.lastRefreshTs) < failuresWindowDuration {
		return
	}

	o.snmpGetOptimizer.refreshFailures()
	o.snmpGetBulkOptimizer.refreshFailures()
	o.snmpGetNextOptimizer.refreshFailures()

	o.lastRefreshTs = now

	log.Debug("SNMP batch size optimizers have been refreshed")
}

// newOidBatchSizeOptimizer creates a oidBatchSizeOptimizer
func newOidBatchSizeOptimizer(snmpOperation snmpOperation, configBatchSize int) *oidBatchSizeOptimizer {
	return &oidBatchSizeOptimizer{
		snmpOperation:       snmpOperation,
		configBatchSize:     configBatchSize,
		batchSize:           configBatchSize,
		failuresByBatchSize: make(map[int]int),
	}
}

// refreshFailures refreshes the failures count for each batch size in oidBatchSizeOptimizer
func (o *oidBatchSizeOptimizer) refreshFailures() {
	o.failuresByBatchSize = make(map[int]int)
}

// onBatchSizeFailure decreases the batch size and returns whether the batch size changed
func (o *oidBatchSizeOptimizer) onBatchSizeFailure() bool {
	o.failuresByBatchSize[o.batchSize]++

	oldBatchSize := o.batchSize

	newBatchSize := max(o.batchSize/onFailureDecreaseFactor, 1)
	if oldBatchSize > o.lastSuccessfulBatchSize && newBatchSize < o.lastSuccessfulBatchSize {
		newBatchSize = o.lastSuccessfulBatchSize
	}

	o.batchSize = newBatchSize

	log.Debugf("SNMP fetch using %s with batch size %d failed, new batch size is %d",
		o.snmpOperation, oldBatchSize, newBatchSize)

	return oldBatchSize != newBatchSize
}

// onBatchSizeSuccess increases the batch size
func (o *oidBatchSizeOptimizer) onBatchSizeSuccess() {
	o.lastSuccessfulBatchSize = o.batchSize

	if o.batchSize >= o.maxBatchSize() {
		return
	}

	newBatchSize := min(o.batchSize+onSuccessIncreaseValue, o.maxBatchSize())
	if o.failuresByBatchSize[newBatchSize] >= maxFailuresPerWindow {
		return
	}

	log.Debugf("SNMP fetch using %s with batch size %d success, new batch size is %d",
		o.snmpOperation, o.lastSuccessfulBatchSize, newBatchSize)

	o.batchSize = newBatchSize
}

// maxBatchSize returns the max batch size
func (o *oidBatchSizeOptimizer) maxBatchSize() int {
	return o.configBatchSize
}
