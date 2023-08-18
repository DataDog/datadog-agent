// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package retry

import (
	"reflect"
	"time"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

// QueueDurationCapacity provides a method to know how much data (express as a duration)
// the in-memory retry queue and the disk storage retry queue can store.
// For each domain, the capacity in bytes is the sum of:
// - the in-memory retry queue capacity. We assume there is enough memory for all in-memory retry queues (one by domain)
// - the available disk storage * `domain relative speed` where `domain relative speed` is the number of
// bytes per second for this domain, divided by the total number of bytes per second for all domains. If a domain receives
// twice the traffic compared to anoter one, twice disk storage capacity is allocated to this domain. Disk storage is shared
// across domain.
// If there is no traffic during the time period for a domain, no statistic is reported.
type QueueDurationCapacity struct {
	accumulators      map[string]*timeIntervalAccumulator
	optionalDiskSpace diskSpace
	maxMemSizeInBytes int
	historyDuration   time.Duration
	bucketDuration    time.Duration
}

type diskSpace interface {
	computeAvailableSpace(extraSize int64) (int64, error)
}

// NewQueueDurationCapacity creates a new instance of *QueueDurationCapacity.
// if `optionalDiskSpace` is not nil, the capacity also use the storage on disk.
// `historyDuration` is the duration used to compute the number of bytes received per second.
// `bucketDuration` is the size of a bucket.
func NewQueueDurationCapacity(
	historyDuration time.Duration,
	bucketDuration time.Duration,
	maxMemSizeInBytes int,
	optionalDiskSpace diskSpace) *QueueDurationCapacity {
	if optionalDiskSpace != nil && reflect.ValueOf(optionalDiskSpace).IsNil() {
		optionalDiskSpace = nil
	}

	return &QueueDurationCapacity{
		accumulators:      make(map[string]*timeIntervalAccumulator),
		maxMemSizeInBytes: maxMemSizeInBytes,
		optionalDiskSpace: optionalDiskSpace,
		historyDuration:   historyDuration,
		bucketDuration:    bucketDuration,
	}
}

// OnTransaction must be called for each transaction.
// Note: because of alternateDomains, `mainDomain` is not necessary the same as transaction.Domain.
func (r *QueueDurationCapacity) OnTransaction(
	transaction *transaction.HTTPTransaction,
	mainDomain string,
	now time.Time) error {

	var accumulator *timeIntervalAccumulator
	var found bool
	if accumulator, found = r.accumulators[mainDomain]; !found {
		var err error
		accumulator, err = newTimeIntervalAccumulator(r.historyDuration, r.bucketDuration)
		if err != nil {
			return err
		}
		r.accumulators[mainDomain] = accumulator
	}

	accumulator.add(now, int64(transaction.GetPayloadSize()))
	return nil
}

// QueueCapacityStats represents statistics about the capacity of the retry queue.
type QueueCapacityStats struct {
	Capacity       time.Duration
	BytesPerSec    float64
	AvailableSpace int64
}

// ComputeCapacity computes the capacity of the retry queue express as a duration.
// Return statistics by domain name.
func (r *QueueDurationCapacity) ComputeCapacity(t time.Time) (map[string]QueueCapacityStats, error) {
	diskSpace, err := r.getTotalDiskSpace()
	if err != nil {
		return nil, err
	}

	var totalBytesPerSec float64
	speedRateByDomain := make(map[string]float64)
	for domain, accumulator := range r.accumulators {
		speedRate := getSpeedRate(accumulator, t)
		totalBytesPerSec += speedRate
		speedRateByDomain[domain] = speedRate
	}

	durations := make(map[string]QueueCapacityStats)
	for domain, bytesPerSec := range speedRateByDomain {
		// If there is no traffic during the time period do not report statistics.
		if bytesPerSec > 0 {
			availableSpace := r.getAvailableSpace(bytesPerSec, totalBytesPerSec, float64(diskSpace))
			durations[domain] = QueueCapacityStats{
				Capacity:       time.Duration(availableSpace/bytesPerSec) * time.Second,
				BytesPerSec:    bytesPerSec,
				AvailableSpace: int64(availableSpace),
			}
		}
	}

	return durations, nil
}

func (r *QueueDurationCapacity) getAvailableSpace(bytesPerSec float64, totalBytesPerSec float64, diskAvailableSpace float64) float64 {
	relativeSpeedRate := bytesPerSec / totalBytesPerSec
	domainDiskSpace := diskAvailableSpace * relativeSpeedRate
	return domainDiskSpace + float64(r.maxMemSizeInBytes)
}

func (r *QueueDurationCapacity) getTotalDiskSpace() (int64, error) {
	var availableSpace int64
	if r.optionalDiskSpace != nil {
		var err error
		availableSpace, err = r.optionalDiskSpace.computeAvailableSpace(0)
		if err != nil {
			return 0, err
		}

		// if capacity is exceeded, availableSpace may be negative.
		if availableSpace < 0 {
			availableSpace = 0
		}
	}
	return availableSpace, nil
}

func getSpeedRate(accumulator *timeIntervalAccumulator, t time.Time) float64 {
	bytes, duration := accumulator.getDuration(t)
	if duration <= 0 {
		return 0
	}
	return float64(bytes) / duration.Seconds()
}
