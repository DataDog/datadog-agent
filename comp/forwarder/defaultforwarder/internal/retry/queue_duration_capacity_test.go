// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package retry

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
)

func TestQueueDurationCapacityMemOnly(t *testing.T) {
	r := require.New(t)
	maxMemSize := 20
	capacity := NewQueueDurationCapacity(time.Second*10, time.Second, maxMemSize, nil)

	addTransaction(r, capacity, "domain", 5, 1)
	addTransaction(r, capacity, "domain", 15, 2)
	stats, err := capacity.ComputeCapacity(time.Unix(2, 0))
	r.NoError(err)
	domainStats, found := stats["domain"]
	r.True(found)

	r.Equal(float64(10), domainStats.BytesPerSec)
	r.Equal(int64(maxMemSize), domainStats.AvailableSpace)
	r.Equal(time.Second*time.Duration(maxMemSize/10), domainStats.Capacity)
}

func TestQueueDurationCapacityMemAndDisk(t *testing.T) {
	r := require.New(t)
	maxMemSize := 20
	mock := &diskSpaceAvailabilityMock{space: 50}
	capacity := NewQueueDurationCapacity(time.Second*10, time.Second, maxMemSize, mock)

	addTransaction(r, capacity, "domain", 20, 1)
	stats, err := capacity.ComputeCapacity(time.Unix(2, 0))
	r.NoError(err)
	domainStats, found := stats["domain"]
	r.True(found)

	r.Equal(float64(10), domainStats.BytesPerSec)
	r.Equal(int64(20+50), domainStats.AvailableSpace)
	r.Equal(time.Second*time.Duration((20+50)/10), domainStats.Capacity)
}

func TestQueueDurationCapacitySeveralDomains(t *testing.T) {
	r := require.New(t)
	maxMemSize := 20
	mock := &diskSpaceAvailabilityMock{space: 50}
	capacity := NewQueueDurationCapacity(time.Second*10, time.Second, maxMemSize, mock)

	addTransaction(r, capacity, "domain1", 5, 1)
	addTransaction(r, capacity, "domain2", 3, 1)
	addTransaction(r, capacity, "domain3", 2, 1)
	totalPayloadSize := float64(5 + 3 + 2)
	stats, err := capacity.ComputeCapacity(time.Unix(2, 0))
	r.NoError(err)
	r.Len(stats, 3)

	r.Equal(float64(5)/2, stats["domain1"].BytesPerSec)
	r.Equal(int64(20+50*5/totalPayloadSize), stats["domain1"].AvailableSpace)

	r.Equal(float64(3)/2, stats["domain2"].BytesPerSec)
	r.Equal(int64(20+50*3/totalPayloadSize), stats["domain2"].AvailableSpace)

	r.Equal(float64(2)/2, stats["domain3"].BytesPerSec)
	r.Equal(int64(20+50*2/totalPayloadSize), stats["domain3"].AvailableSpace)
}

func TestQueueDurationCapacitEmptyTraffic(t *testing.T) {
	r := require.New(t)
	maxMemSize := 20
	capacity := NewQueueDurationCapacity(time.Second*10, time.Second, maxMemSize, nil)

	addTransaction(r, capacity, "domain1", 20, 1)
	addTransaction(r, capacity, "domain2", 0, 1)
	stats, err := capacity.ComputeCapacity(time.Unix(1, 0))
	r.NoError(err)
	r.Len(stats, 1)
	_, found := stats["domain1"]
	r.True(found)
}

type diskSpaceAvailabilityMock struct {
	space int64
}

func (m *diskSpaceAvailabilityMock) computeAvailableSpace(_ int64) (int64, error) {
	return m.space, nil
}

func addTransaction(
	r *require.Assertions,
	capacity *QueueDurationCapacity,
	domain string,
	payloadSize int,
	unixTime int64) {

	payload := make([]byte, payloadSize)
	tr := &transaction.HTTPTransaction{Payload: transaction.NewBytesPayloadWithoutMetaData(payload)}
	err := capacity.OnTransaction(tr, domain, time.Unix(unixTime, 0))
	r.NoError(err)
}
