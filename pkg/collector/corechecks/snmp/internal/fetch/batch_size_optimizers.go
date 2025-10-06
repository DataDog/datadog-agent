package fetch

import (
	"time"
)

const (
	increaseValue  = 1
	decreaseFactor = 2

	failuresTimeInterval       = 30 * time.Minute
	maxFailuresPerTimeInterval = 2
)

// OidBatchSizeOptimizers holds oidBatchSizeOptimizer for each SNMP request operation
type OidBatchSizeOptimizers struct {
	snmpGetOptimizer     *oidBatchSizeOptimizer
	snmpGetNextOptimizer *oidBatchSizeOptimizer
	snmpGetBulkOptimizer *oidBatchSizeOptimizer
	lastRefreshTs        time.Time
}

// oidBatchSizeOptimizer holds data between check runs to be able to find an optimized batch size for SNMP requests
type oidBatchSizeOptimizer struct {
	configBatchSize     int
	batchSize           int
	failuresByBatchSize map[int]int
}

// NewOidBatchSizeOptimizers creates a OidBatchSizeOptimizers
func NewOidBatchSizeOptimizers(configBatchSize int) *OidBatchSizeOptimizers {
	return &OidBatchSizeOptimizers{
		snmpGetOptimizer:     newOidBatchSizeOptimizer(configBatchSize),
		snmpGetNextOptimizer: newOidBatchSizeOptimizer(configBatchSize),
		snmpGetBulkOptimizer: newOidBatchSizeOptimizer(configBatchSize),
	}
}

// isOutdated returns whether OidBatchSizeOptimizers should be refreshed
func (o *OidBatchSizeOptimizers) isOutdated() bool {
	return time.Since(o.lastRefreshTs) > failuresTimeInterval
}

// Refresh refreshes the failures count for each oidBatchSizeOptimizer in OidBatchSizeOptimizers
func (o *OidBatchSizeOptimizers) refreshFailuresCount() {
	o.snmpGetOptimizer.failuresByBatchSize = make(map[int]int)
	o.snmpGetNextOptimizer.failuresByBatchSize = make(map[int]int)
	o.snmpGetBulkOptimizer.failuresByBatchSize = make(map[int]int)
}

// newOidBatchSizeOptimizer creates a oidBatchSizeOptimizer
func newOidBatchSizeOptimizer(configBatchSize int) *oidBatchSizeOptimizer {
	return &oidBatchSizeOptimizer{
		configBatchSize:     configBatchSize,
		batchSize:           configBatchSize,
		failuresByBatchSize: make(map[int]int),
	}
}

// onBatchSizeFailure decreases the batch size and returns whether the old batch size was already the min batch size
func (o *oidBatchSizeOptimizer) onBatchSizeFailure() bool {
	o.failuresByBatchSize[o.batchSize]++

	if o.batchSize <= o.minBatchSize() {
		return false
	}

	o.batchSize = max(o.batchSize/decreaseFactor, o.minBatchSize())

	return true
}

// onBatchSizeSuccess increases the batch size
func (o *oidBatchSizeOptimizer) onBatchSizeSuccess() {
	if o.batchSize >= o.maxBatchSize() {
		return
	}

	newBatchSize := min(o.batchSize+increaseValue, o.maxBatchSize())
	if o.failuresByBatchSize[newBatchSize] > maxFailuresPerTimeInterval {
		return
	}

	o.batchSize = newBatchSize
}

// minBatchSize returns the min batch size
func (o *oidBatchSizeOptimizer) minBatchSize() int {
	return 1
}

// maxBatchSize returns the max batch size
func (o *oidBatchSizeOptimizer) maxBatchSize() int {
	return o.configBatchSize
}
