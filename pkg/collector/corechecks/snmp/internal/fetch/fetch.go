// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fetch

import (
	"fmt"
	"strconv"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

type columnFetchStrategy int

const (
	useGetBulk columnFetchStrategy = iota
	useGetNext
)

func (c columnFetchStrategy) String() string {
	switch c {
	case useGetBulk:
		return "useGetBulk"
	case useGetNext:
		return "useGetNext"
	default:
		return strconv.Itoa(int(c))
	}
}

// Fetch oid values from device
func Fetch(sess session.Session, scalarOIDs, columnOIDs []string, batchSizeOptimizers *OidBatchSizeOptimizers,
	bulkMaxRepetitions uint32) (*valuestore.ResultValueStore, error) {
	now := time.Now()

	batchSizeOptimizers.refreshIfOutdated(now)

	// fetch scalar values
	scalarResults, err := fetchScalarOidsWithBatching(sess, scalarOIDs, batchSizeOptimizers.snmpGetOptimizer)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scalar oids with batching: %v", err)
	}

	columnResults, err := fetchColumnOidsWithBatching(sess, columnOIDs, batchSizeOptimizers.snmpGetBulkOptimizer,
		bulkMaxRepetitions, useGetBulk)
	if err != nil {
		log.Debugf("failed to fetch oids with GetBulk batching: %v", err)

		columnResults, err = fetchColumnOidsWithBatching(sess, columnOIDs, batchSizeOptimizers.snmpGetNextOptimizer,
			bulkMaxRepetitions, useGetNext)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch oids with GetNext batching: %v", err)
		}
	}

	return &valuestore.ResultValueStore{ScalarValues: scalarResults, ColumnValues: columnResults}, nil
}
