// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fetch

import (
	"fmt"
	"sort"

	"github.com/cihub/seelog"
	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
)

func fetchColumnOidsWithBatching(sess session.Session, oids map[string]string, oidBatchSize int, bulkMaxRepetitions uint32, fetchStrategy columnFetchStrategy) (valuestore.ColumnResultValuesType, error) {
	retValues := make(valuestore.ColumnResultValuesType, len(oids))

	columnOids := getOidsMapKeys(oids)
	sort.Strings(columnOids) // sorting ColumnOids to make them deterministic for testing purpose
	batches, err := common.CreateStringBatches(columnOids, oidBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create column oid batches: %s", err)
	}

	for _, batchColumnOids := range batches {
		oidsToFetch := make(map[string]string, len(batchColumnOids))
		for _, oid := range batchColumnOids {
			oidsToFetch[oid] = oids[oid]
		}

		results, err := fetchColumnOids(sess, oidsToFetch, bulkMaxRepetitions, fetchStrategy)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch column oids: %s", err)
		}

		for columnOid, instanceOids := range results {
			if _, ok := retValues[columnOid]; !ok {
				retValues[columnOid] = instanceOids
				continue
			}
			for oid, value := range instanceOids {
				retValues[columnOid][oid] = value
			}
		}
	}
	return retValues, nil
}

// fetchColumnOids has an `oids` argument representing a `map[string]string`,
// the key of the map is the column oid, and the value is the oid used to fetch the next value for the column.
// The value oid might be equal to column oid or a row oid of the same column.
func fetchColumnOids(sess session.Session, oids map[string]string, bulkMaxRepetitions uint32, fetchStrategy columnFetchStrategy) (valuestore.ColumnResultValuesType, error) {
	returnValues := make(valuestore.ColumnResultValuesType, len(oids))
	alreadyProcessedOids := make(map[string]bool)
	curOids := oids
	for {
		if len(curOids) == 0 {
			break
		}
		log.Debugf("fetch column: request oids (maxRep:%d,fetchStrategy:%s): %v", bulkMaxRepetitions, fetchStrategy, curOids)
		var columnOids, requestOids []string
		for k, v := range curOids {
			if alreadyProcessedOids[v] {
				log.Debugf("fetch column: OID already processed: %s", v)
				continue
			}
			alreadyProcessedOids[v] = true
			columnOids = append(columnOids, k)
			requestOids = append(requestOids, v)
		}
		if len(columnOids) == 0 {
			break
		}
		// sorting ColumnOids and requestOids to make them deterministic for testing purpose
		sort.Strings(columnOids)
		sort.Strings(requestOids)

		results, err := getResults(sess, requestOids, bulkMaxRepetitions, fetchStrategy)
		if err != nil {
			return nil, err
		}
		newValues, nextOids := valuestore.ResultToColumnValues(columnOids, results)
		updateColumnResultValues(returnValues, newValues)
		curOids = nextOids
	}
	return returnValues, nil
}

func getResults(sess session.Session, requestOids []string, bulkMaxRepetitions uint32, fetchStrategy columnFetchStrategy) (*gosnmp.SnmpPacket, error) {
	var results *gosnmp.SnmpPacket
	if sess.GetVersion() == gosnmp.Version1 || fetchStrategy == useGetNext {
		// snmp v1 doesn't support GetBulk
		getNextResults, err := sess.GetNext(requestOids)
		if err != nil {
			log.Debugf("fetch column: failed getting oids `%v` using GetNext: %s", requestOids, err)
			return nil, fmt.Errorf("fetch column: failed getting oids `%v` using GetNext: %s", requestOids, err)
		}
		results = getNextResults
		if log.ShouldLog(seelog.DebugLvl) {
			log.Debugf("fetch column: GetNext results: %v", gosnmplib.PacketAsString(results))
		}
	} else {
		getBulkResults, err := sess.GetBulk(requestOids, bulkMaxRepetitions)
		if err != nil {
			log.Debugf("fetch column: failed getting oids `%v` using GetBulk: %s", requestOids, err)
			return nil, fmt.Errorf("fetch column: failed getting oids `%v` using GetBulk: %s", requestOids, err)
		}
		results = getBulkResults
		if log.ShouldLog(seelog.DebugLvl) {
			log.Debugf("fetch column: GetBulk results: %v", gosnmplib.PacketAsString(results))
		}
	}
	return results, nil
}

func updateColumnResultValues(valuesToUpdate valuestore.ColumnResultValuesType, extraValues valuestore.ColumnResultValuesType) {
	for columnOid, columnValues := range extraValues {
		for oid, value := range columnValues {
			if _, ok := valuesToUpdate[columnOid]; !ok {
				valuesToUpdate[columnOid] = make(map[string]valuestore.ResultValue)
			}
			valuesToUpdate[columnOid][oid] = value
		}
	}
}

func getOidsMapKeys(oidsMap map[string]string) []string {
	keys := make([]string, len(oidsMap))
	i := 0
	for k := range oidsMap {
		keys[i] = k
		i++
	}
	return keys
}
