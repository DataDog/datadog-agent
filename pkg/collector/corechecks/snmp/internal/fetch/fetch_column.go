// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fetch

import (
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/gosnmp/gosnmp"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/common"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/session"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/valuestore"
	"github.com/DataDog/datadog-agent/pkg/snmp/gosnmplib"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type nonIncreasingOidError struct {
	oids []string
}

func newNonIncreasingOidError(oids []string) *nonIncreasingOidError {
	oids = sortedUniqueOids(oids)
	return &nonIncreasingOidError{oids: oids}
}

func (e *nonIncreasingOidError) Error() string {
	return fmt.Sprintf("non-increasing OID response detected for OIDs: %s", strings.Join(e.oids, ", "))
}

func fetchColumnOidsWithBatching(sess session.Session, oids []string, batchSizeOptimizer *oidBatchSizeOptimizer, bulkMaxRepetitions uint32, fetchStrategy columnFetchStrategy, ignoreNonIncreasingOid bool, deviceAddress string) (valuestore.ColumnResultValuesType, error) {
	retValues := make(valuestore.ColumnResultValuesType, len(oids))
	if len(oids) == 0 {
		return retValues, nil
	}

	batches, err := common.CreateStringBatches(oids, batchSizeOptimizer.batchSize)
	if err != nil {
		return nil, fmt.Errorf("failed to create column oid batches: %s", err)
	}

	for _, batchColumnOids := range batches {
		results, err := fetchColumnOids(sess, batchColumnOids, bulkMaxRepetitions, fetchStrategy, ignoreNonIncreasingOid, deviceAddress)
		for columnOid, instanceOids := range results {
			if _, ok := retValues[columnOid]; !ok {
				retValues[columnOid] = instanceOids
				continue
			}
			maps.Copy(retValues[columnOid], instanceOids)
		}
		if err != nil {
			var fetchErr *fetchError
			if errors.As(err, &fetchErr) {
				shouldRetry := batchSizeOptimizer.onBatchSizeFailure()
				if shouldRetry {
					return fetchColumnOidsWithBatching(sess, oids, batchSizeOptimizer, bulkMaxRepetitions, fetchStrategy, ignoreNonIncreasingOid, deviceAddress)
				}
			}

			var nonIncreasingErr *nonIncreasingOidError
			if errors.As(err, &nonIncreasingErr) {
				return retValues, fmt.Errorf("failed to fetch column oids: %w", err)
			}
			return nil, fmt.Errorf("failed to fetch column oids: %w", err)
		}
	}

	batchSizeOptimizer.onBatchSizeSuccess()

	return retValues, nil
}

// fetchColumnOids fetches all values for each specified column OID.
// bulkMaxRepetitions is the number of entries to request per OID per SNMP
// request when fetchStrategy = useGetBulk; it is ignored when fetchStrategy is
// useGetNext.
func fetchColumnOids(sess session.Session, oids []string, bulkMaxRepetitions uint32, fetchStrategy columnFetchStrategy, ignoreNonIncreasingOid bool, deviceAddress string) (valuestore.ColumnResultValuesType, error) {
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
		var repeatedOids []string
		for k, v := range curOids {
			if alreadyProcessedOids[v] {
				log.Debugf("fetch column: OID already processed: %s", v)
				repeatedOids = append(repeatedOids, v)
				continue
			}
			alreadyProcessedOids[v] = true
			columnOids = append(columnOids, k)
			requestOids = append(requestOids, v)
		}
		if len(repeatedOids) > 0 {
			warnNonIncreasingOids(deviceAddress, repeatedOids)
			if !ignoreNonIncreasingOid {
				return returnValues, newNonIncreasingOidError(repeatedOids)
			}
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
		newValues, nextOids, nonMonotonicOids := valuestore.ResultToColumnValues(columnOids, results)
		updateColumnResultValues(returnValues, newValues)
		if len(nonMonotonicOids) > 0 {
			warnNonIncreasingOids(deviceAddress, nonMonotonicOids)
			if !ignoreNonIncreasingOid {
				return returnValues, newNonIncreasingOidError(nonMonotonicOids)
			}
		}
		curOids = nextOids
	}
	return returnValues, nil
}

func warnNonIncreasingOids(deviceAddress string, oids []string) {
	oids = sortedUniqueOids(oids)
	if deviceAddress == "" {
		log.Warnf("SNMP device returned non-increasing OIDs; stopping affected column walks for OIDs: %s", strings.Join(oids, ", "))
		return
	}
	log.Warnf("SNMP device %s returned non-increasing OIDs; stopping affected column walks for OIDs: %s", deviceAddress, strings.Join(oids, ", "))
}

func sortedUniqueOids(oids []string) []string {
	oids = append([]string(nil), oids...)
	sort.Strings(oids)
	return slicesCompact(oids)
}

func slicesCompact(values []string) []string {
	if len(values) < 2 {
		return values
	}
	writeIndex := 1
	for readIndex := 1; readIndex < len(values); readIndex++ {
		if values[readIndex] == values[readIndex-1] {
			continue
		}
		values[writeIndex] = values[readIndex]
		writeIndex++
	}
	return values[:writeIndex]
}

func getResults(sess session.Session, requestOids []string, bulkMaxRepetitions uint32, fetchStrategy columnFetchStrategy) (*gosnmp.SnmpPacket, error) {
	if sess.GetVersion() == gosnmp.Version1 && fetchStrategy == useGetBulk {
		// snmp v1 doesn't support GetBulk
		return nil, errors.New("GetBulk not supported in SNMP v1")
	}

	var results *gosnmp.SnmpPacket
	if fetchStrategy == useGetNext {
		getNextResults, err := sess.GetNext(requestOids)
		if err != nil {
			fetchErr := newFetchError(columnOid, requestOids, snmpGetNext, err)
			log.Debug(fetchErr.Error())
			return nil, fetchErr
		}
		results = getNextResults
		if log.ShouldLog(log.TraceLvl) {
			log.Tracef("fetch column: GetNext results: %v", gosnmplib.PacketAsString(results))
		}
	} else {
		getBulkResults, err := sess.GetBulk(requestOids, bulkMaxRepetitions)
		if err != nil {
			fetchErr := newFetchError(columnOid, requestOids, snmpGetBulk, err)
			log.Debug(fetchErr.Error())
			return nil, fetchErr
		}

		// Some devices truncate GetBulk responses without returning an SNMP
		// error. Treat that as a fetch failure so batching retries with a
		// smaller request instead of silently losing metrics.
		if len(getBulkResults.Variables) < len(requestOids) {
			err := fmt.Errorf("response truncated: got %d varbinds for %d OIDs", len(getBulkResults.Variables), len(requestOids))
			fetchErr := newFetchError(columnOid, requestOids, snmpGetBulk, err)
			log.Debug(fetchErr.Error())
			return nil, fetchErr
		}
		results = getBulkResults
		if log.ShouldLog(log.TraceLvl) {
			log.Tracef("fetch column: GetBulk results: %v", gosnmplib.PacketAsString(results))
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
