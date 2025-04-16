// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fetch

import (
	"fmt"
	"maps"
	"sort"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func fetchColumnOidsWithBatching(sess session.Session, oids []string, oidBatchSize int, bulkMaxRepetitions uint32, fetchStrategy columnFetchStrategy) (valuestore.ColumnResultValuesType, error) {
	retValues := make(valuestore.ColumnResultValuesType, len(oids))

	batches, err := common.CreateStringBatches(oids, oidBatchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create column oid batches: %s", err)
	}

	for _, batchColumnOids := range batches {
		results, err := fetchColumnOids(sess, batchColumnOids, bulkMaxRepetitions, fetchStrategy)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch column oids: %s", err)
		}

		for columnOid, instanceOids := range results {
			if _, ok := retValues[columnOid]; !ok {
				retValues[columnOid] = instanceOids
				continue
			}
			maps.Copy(retValues[columnOid], instanceOids)
		}
	}
	return retValues, nil
}

// fetchColumnOids fetches all values for each specified column OID.
// bulkMaxRepetitions is the number of entries to request per OID per SNMP
// request when fetchStrategy = useGetBulk; it is ignored when fetchStrategy is
// useGetNext.
func fetchColumnOids(sess session.Session, oids []string, bulkMaxRepetitions uint32, fetchStrategy columnFetchStrategy) (valuestore.ColumnResultValuesType, error) {
	returnValues := make(valuestore.ColumnResultValuesType, len(oids))
	alreadyProcessedOids := make(map[string]bool)
	curOids := make(map[string]string, len(oids))
	for _, oid := range oids {
		curOids[oid] = oid
	}
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
		if log.ShouldLog(log.DebugLvl) {
			log.Debugf("fetch column: GetNext results: %v", gosnmplib.PacketAsString(results))
		}
	} else {
		getBulkResults, err := sess.GetBulk(requestOids, bulkMaxRepetitions)
		if err != nil {
			log.Debugf("fetch column: failed getting oids `%v` using GetBulk: %s", requestOids, err)
			return nil, fmt.Errorf("fetch column: failed getting oids `%v` using GetBulk: %s", requestOids, err)
		}
		results = getBulkResults
		if log.ShouldLog(log.DebugLvl) {
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
