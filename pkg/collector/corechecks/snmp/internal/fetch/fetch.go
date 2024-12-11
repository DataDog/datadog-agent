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

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/checkconfig"
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
// TODO: pass only specific configs instead of the whole CheckConfig
func Fetch(sess session.Session, config *checkconfig.CheckConfig) (*valuestore.ResultValueStore, error) {
	// fetch scalar values
	scalarResults, err := fetchScalarOidsWithBatching(sess, config.OidConfig.ScalarOids, config.OidBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch scalar oids with batching: %v", err)
	}

	// fetch column values
	oids := make(map[string]string, len(config.OidConfig.ColumnOids))
	for _, value := range config.OidConfig.ColumnOids {
		oids[value] = value
	}

	columnResults, err := fetchColumnOidsWithBatching(sess, oids, config.OidBatchSize, config.BulkMaxRepetitions, useGetBulk)
	if err != nil {
		log.Debugf("failed to fetch oids with GetBulk batching: %v", err)

		columnResults, err = fetchColumnOidsWithBatching(sess, oids, config.OidBatchSize, config.BulkMaxRepetitions, useGetNext)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch oids with GetNext batching: %v", err)
		}
	}

	return &valuestore.ResultValueStore{ScalarValues: scalarResults, ColumnValues: columnResults}, nil
}
