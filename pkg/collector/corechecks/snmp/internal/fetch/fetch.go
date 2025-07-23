// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(NDM) Fix revive linter
package fetch

import (
	"fmt"
	"strconv"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

type columnFetchStrategy int

const (
	UseGetBulk columnFetchStrategy = iota
	UseGetNext
)

func (c columnFetchStrategy) String() string {
	switch c {
	case UseGetBulk:
		return "UseGetBulk"
	case UseGetNext:
		return "UseGetNext"
	default:
		return strconv.Itoa(int(c))
	}
}

// Fetch oid values from device
func Fetch(sess session.Session, scalarOIDs, columnOIDs []string, batchSize int,
	bulkMaxRepetitions uint32) (*valuestore.ResultValueStore, error) {
	// fetch scalar values
	scalarResults, err := fetchScalarOidsWithBatching(sess, scalarOIDs, batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scalar oids with batching: %v", err)
	}

	columnResults, err := FetchColumnOidsWithBatching(sess, columnOIDs, batchSize,
		bulkMaxRepetitions, UseGetBulk)
	if err != nil {
		log.Debugf("failed to fetch oids with GetBulk batching: %v", err)

		columnResults, err = FetchColumnOidsWithBatching(sess, columnOIDs, batchSize, bulkMaxRepetitions,
			UseGetNext)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch oids with GetNext batching: %v", err)
		}
	}

	return &valuestore.ResultValueStore{ScalarValues: scalarResults, ColumnValues: columnResults}, nil
}
