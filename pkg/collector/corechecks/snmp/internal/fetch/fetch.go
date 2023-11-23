// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fetch

import (
	"fmt"
	"github.com/gosnmp/gosnmp"
	"strconv"
	"time"

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

const defaultDeviceScanRootOid = "1.0"

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

	results := getDeviceScanValues(sess, config)

	return &valuestore.ResultValueStore{ScalarValues: scalarResults, ColumnValues: columnResults, DeviceScanValues: results}, nil
}

func getDeviceScanValues(sess session.Session, config *checkconfig.CheckConfig) []gosnmp.SnmpPDU {
	// TODO: Use a internal type instead of gosnmp.SnmpPDU to avoid leaking gosnmp types ?
	var results []gosnmp.SnmpPDU
	if config.DeviceScanEnabled {
		rootOid := config.DeviceScanLastOid // default root Oid
		if rootOid == "" {
			// NEW DEVICE SCAN
			rootOid = defaultDeviceScanRootOid
			config.DeviceScanCurScanStart = time.Now()
			config.DeviceScanCurScanOidsCount = 0
		}

		maxOidsToFetch := 100 // TODO: Update to 1000 (?)
		fetchStart := time.Now()
		fetchedResults, lastOid, err := session.FetchAllOIDsUsingGetNext(sess, rootOid, maxOidsToFetch)
		if err != nil {
			log.Warnf("[FetchAllOIDsUsingGetNext] error: %s", err)
			return nil
		}
		fetchDuration := time.Since(fetchStart)
		log.Warnf("[FetchAllOIDsUsingGetNext] PRINT PDUs (len: %d)", len(results))
		for _, resultPdu := range fetchedResults {
			log.Warnf("[FetchAllOIDsUsingGetNext] PDU: %+v", resultPdu)
		}
		config.DeviceScanCurScanOidsCount += len(fetchedResults)

		if len(fetchedResults) == maxOidsToFetch {
			log.Warnf("[FetchAllOIDsUsingGetNext] Partial Device Scan (Total Count: %d, Fetch Duration Ms: %d)",
				config.DeviceScanCurScanOidsCount,
				fetchDuration.Milliseconds(),
			)
			// Partial Device Scan
			config.DeviceScanLastOid = lastOid
		} else {
			log.Warnf("[FetchAllOIDsUsingGetNext] Full Device Scan (Total Count: %d, Duration: %.2f Sec)",
				config.DeviceScanCurScanOidsCount,
				time.Since(config.DeviceScanCurScanStart).Seconds(),
			)
			// Full Device Scan completed
			config.DeviceScanLastOid = ""
		}
		results = fetchedResults
	}
	return results
}
